package mail

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// ensureIdentity gives a non-guest user one personal inbox and one identity.
// It adopts the inbox the user already owns (every user created before this
// release has one, unless it was created before any domain was verified or
// has an unusable username) and otherwise creates an inbox on the main domain. It
// returns nil, nil for a guest, and when no domain is verified yet: the
// domain-verified hook calls ensureIdentities, so the user is picked up then.
func ensureIdentity(app core.App, user *core.Record) (*core.Record, error) {
	if user.GetString("role") == "guest" {
		return nil, nil
	}
	existing, err := app.FindFirstRecordByFilter("mail_identities", "user = {:user}", dbx.Params{"user": user.Id})
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("identity lookup: %w", err)
	}

	inbox, err := adoptPersonalInbox(app, user.Id)
	if err != nil {
		return nil, err
	}
	var emailName string
	if inbox != nil {
		// Legacy inbox addresses were never pattern-checked.
		emailName = normalizeLocalPart(inbox.GetString("address"))
		taken := true
		if localPartPattern.MatchString(emailName) {
			taken, err = emailNameTaken(app, emailName, inbox.Id)
			if err != nil {
				return nil, err
			}
		}
		if taken {
			emailName = deriveEmailName(app, user.GetString("username"), inbox.GetString("domain"))
		}
	} else {
		domain, err := mainDomain(app)
		if err != nil || domain == nil {
			return nil, err
		}
		emailName = deriveEmailName(app, user.GetString("username"), domain.Id)
		if emailName != "" {
			inbox, err = createPersonalInbox(app, user, emailName, domain.Id)
			if err != nil {
				return nil, err
			}
		}
	}
	// Deliberate: a user we cannot name gets no identity rather than a failed
	// account creation; an admin can fix the username. Error, not Warn: the
	// user is silently without mail until someone notices.
	if emailName == "" {
		addressLog.Error("could not choose an email name; the user has no mail identity",
			"user", user.Id, "username", user.GetString("username"))
		return nil, nil
	}

	col, err := app.FindCollectionByNameOrId("mail_identities")
	if err != nil {
		return nil, err
	}
	identity := core.NewRecord(col)
	identity.Set("user", user.Id)
	identity.Set("inbox", inbox.Id)
	identity.Set("email_name", emailName)
	if err := app.Save(identity); err != nil {
		return nil, fmt.Errorf("save identity: %w", err)
	}
	return identity, nil
}

// ensureIdentities runs ensureIdentity for every non-guest user. One user's
// failure does not stop the others; the errors are joined.
func ensureIdentities(app core.App) error {
	users, err := app.FindRecordsByFilter("users", "role != 'guest'", "created", 0, 0)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	var errs []error
	for _, user := range users {
		if _, err := ensureIdentity(app, user); err != nil {
			errs = append(errs, fmt.Errorf("user %s: %w", user.Id, err))
		}
	}
	return errors.Join(errs...)
}

// adoptPersonalInbox returns the personal inbox the user already owns and that
// no identity claims yet, or nil.
func adoptPersonalInbox(app core.App, userID string) (*core.Record, error) {
	ids, err := findPersonalMailboxesOf(app, userID)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		_, err := app.FindFirstRecordByFilter("mail_identities", "inbox = {:inbox}", dbx.Params{"inbox": id})
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("identity lookup for inbox %s: %w", id, err)
		}
		return app.FindRecordById("mail_mailboxes", id)
	}
	return nil, nil
}

// mainDomain is the verified domain new inboxes are created on: the first by
// the admin's order (rank), then the oldest.
func mainDomain(app core.App) (*core.Record, error) {
	rows, err := app.FindRecordsByFilter("mail_domains", "verified = true", "rank,created", 1, 0)
	if err != nil {
		return nil, fmt.Errorf("find main domain: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

// emailNameTaken reports whether name cannot be a new identity's email name:
// another identity has it, or it is already an address of a different inbox
// on a domain with a rule (where the name would be handed out).
func emailNameTaken(app core.App, name, inboxID string) (bool, error) {
	if _, err := app.FindFirstRecordByFilter("mail_identities", "email_name = {:name}", dbx.Params{"name": name}); err == nil {
		return true, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	rows, err := app.FindRecordsByFilter("mail_addresses",
		"local_part = {:name} && domain.coverage = {:everyone} && mailbox != {:inbox}", "", 1, 0,
		dbx.Params{"name": name, "everyone": coverageEveryone, "inbox": inboxID})
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// deriveEmailName turns a username into a free email name: the sanitized
// username, then name2 … name99. Free means emailNameTaken is false and the
// name is not already an address on the inbox's own domain.
func deriveEmailName(app core.App, username, inboxDomainID string) string {
	base := addressSanitizer.ReplaceAllString(strings.ToLower(strings.TrimSpace(username)), "")
	if base == "" {
		return ""
	}
	for i := 1; i <= 99; i++ {
		suffix := ""
		if i > 1 {
			suffix = fmt.Sprint(i)
		}
		trimmed := base
		if max := 64 - len(suffix); len(trimmed) > max {
			trimmed = trimmed[:max]
		}
		candidate := trimmed + suffix
		if !localPartPattern.MatchString(candidate) {
			continue
		}
		taken, err := emailNameTaken(app, candidate, "")
		if err != nil {
			addressLog.Error("could not check an email name", "name", candidate, "error", err)
			return ""
		}
		if taken {
			continue
		}
		row, err := findAddressRow(app, candidate, inboxDomainID)
		if err != nil {
			addressLog.Error("could not check an address", "name", candidate, "error", err)
			return ""
		}
		if row == nil {
			return candidate
		}
	}
	return ""
}

// createPersonalInbox creates the user's inbox and their owner membership in
// one transaction. The mailbox hooks give it its primary address row.
func createPersonalInbox(app core.App, user *core.Record, emailName, domainID string) (*core.Record, error) {
	var inbox *core.Record
	err := app.RunInTransaction(func(txApp core.App) error {
		mailboxes, err := txApp.FindCollectionByNameOrId("mail_mailboxes")
		if err != nil {
			return err
		}
		inbox = core.NewRecord(mailboxes)
		inbox.Set("address", emailName)
		inbox.Set("domain", domainID)
		inbox.Set("display_name", user.GetString("name"))
		inbox.Set("name", user.GetString("name"))
		inbox.Set("type", "personal")
		if err := txApp.Save(inbox); err != nil {
			return fmt.Errorf("create personal inbox: %w", err)
		}
		members, err := txApp.FindCollectionByNameOrId("mail_mailbox_members")
		if err != nil {
			return err
		}
		member := core.NewRecord(members)
		member.Set("mailbox", inbox.Id)
		member.Set("user", user.Id)
		member.Set("role", "owner")
		return txApp.Save(member)
	})
	if err != nil {
		return nil, err
	}
	return inbox, nil
}

// registerIdentityHooks gives every new or updated non-guest user an identity.
func registerIdentityHooks(app core.App) {
	app.OnRecordAfterCreateSuccess("users").BindFunc(func(e *core.RecordEvent) error {
		if _, err := ensureIdentity(e.App, e.Record); err != nil {
			addressLog.Error("could not give a new user a mail identity", "user", e.Record.Id, "error", err)
		}
		return e.Next()
	})
	// Idempotent, so it needs no old role: it covers a promoted guest and heals
	// a non-guest that lacks an identity.
	app.OnRecordAfterUpdateSuccess("users").BindFunc(func(e *core.RecordEvent) error {
		if _, err := ensureIdentity(e.App, e.Record); err != nil {
			addressLog.Error("could not ensure a mail identity after a user update", "user", e.Record.Id, "error", err)
		}
		return e.Next()
	})
}
