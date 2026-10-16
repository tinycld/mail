package mail

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// ruleScope limits a reconcile to one user or one domain. The zero value
// reconciles everything (boot).
type ruleScope struct {
	UserID   string
	DomainID string
}

type ruleKey struct {
	local  string
	domain string
}

// reconcileRuleAddresses converges the source = 'rule' rows in scope to what
// the domain rules say: each non-guest identity gets <email_name>@<domain> on
// each verified domain whose coverage is everyone. A row that already exists
// for the same address and inbox (the inbox's primary row, or an address an
// admin added) satisfies the rule, and nothing is written. A row held by a
// different inbox is a conflict: it is logged and left alone (the coverage
// guard keeps new conflicts from being made).
func reconcileRuleAddresses(app core.App, scope ruleScope) error {
	expected, err := expectedRuleAddresses(app, scope)
	if err != nil {
		return err
	}
	actual, err := actualRuleAddresses(app, scope)
	if err != nil {
		return err
	}
	col, err := app.FindCollectionByNameOrId("mail_addresses")
	if err != nil {
		return err
	}

	var errs []error
	for key, inbox := range expected {
		if row, ok := actual[key]; ok {
			delete(actual, key)
			if row.GetString("mailbox") != inbox {
				row.Set("mailbox", inbox)
				if err := app.Save(row); err != nil {
					errs = append(errs, err)
				}
			}
			continue
		}
		existing, err := findAddressRow(app, key.local, key.domain)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if existing != nil {
			if existing.GetString("mailbox") != inbox {
				addressLog.Warn("a rule address is held by another inbox",
					"local_part", key.local, "domain", key.domain, "inbox", inbox, "held_by", existing.GetString("mailbox"))
			}
			continue
		}
		row := core.NewRecord(col)
		row.Set("local_part", key.local)
		row.Set("domain", key.domain)
		row.Set("mailbox", inbox)
		row.Set("source", addressSourceRule)
		if err := app.Save(row); err != nil {
			errs = append(errs, fmt.Errorf("add %s on %s: %w", key.local, key.domain, err))
		}
	}
	for _, row := range actual {
		if err := app.Delete(row); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func expectedRuleAddresses(app core.App, scope ruleScope) (map[ruleKey]string, error) {
	domainFilter := "coverage = {:everyone} && verified = true"
	params := dbx.Params{"everyone": coverageEveryone}
	if scope.DomainID != "" {
		domainFilter += " && id = {:domain}"
		params["domain"] = scope.DomainID
	}
	domains, err := app.FindRecordsByFilter("mail_domains", domainFilter, "", 0, 0, params)
	if err != nil {
		return nil, fmt.Errorf("list rule domains: %w", err)
	}

	identityFilter := "user.role != 'guest'"
	identityParams := dbx.Params{}
	if scope.UserID != "" {
		identityFilter += " && user = {:user}"
		identityParams["user"] = scope.UserID
	}
	identities, err := app.FindRecordsByFilter("mail_identities", identityFilter, "", 0, 0, identityParams)
	if err != nil {
		return nil, fmt.Errorf("list identities: %w", err)
	}

	expected := map[ruleKey]string{}
	for _, domain := range domains {
		for _, identity := range identities {
			expected[ruleKey{local: identity.GetString("email_name"), domain: domain.Id}] = identity.GetString("inbox")
		}
	}
	return expected, nil
}

func actualRuleAddresses(app core.App, scope ruleScope) (map[ruleKey]*core.Record, error) {
	filter := "source = {:rule}"
	params := dbx.Params{"rule": addressSourceRule}
	if scope.DomainID != "" {
		filter += " && domain = {:domain}"
		params["domain"] = scope.DomainID
	}
	if scope.UserID != "" {
		identity, err := app.FindFirstRecordByFilter("mail_identities", "user = {:user}", dbx.Params{"user": scope.UserID})
		if err != nil {
			// No identity: its rows went with its inbox.
			return map[ruleKey]*core.Record{}, nil
		}
		filter += " && mailbox = {:inbox}"
		params["inbox"] = identity.GetString("inbox")
	}
	rows, err := app.FindRecordsByFilter("mail_addresses", filter, "", 0, 0, params)
	if err != nil {
		return nil, fmt.Errorf("list rule addresses: %w", err)
	}
	actual := make(map[ruleKey]*core.Record, len(rows))
	for _, row := range rows {
		actual[ruleKey{local: row.GetString("local_part"), domain: row.GetString("domain")}] = row
	}
	return actual, nil
}

// checkCoverageConflicts refuses to turn on a rule for a domain where a
// person's email name is already an address of a different inbox. Each
// conflicting address is named so the admin can rename it first.
func checkCoverageConflicts(app core.App, domain *core.Record) error {
	if !domainHasRule(domain) {
		return nil
	}
	identities, err := app.FindRecordsByFilter("mail_identities", "user.role != 'guest'", "", 0, 0)
	if err != nil {
		return fmt.Errorf("list identities: %w", err)
	}
	var conflicts []string
	for _, identity := range identities {
		row, err := findAddressRow(app, identity.GetString("email_name"), domain.Id)
		if err != nil {
			return err
		}
		if row != nil && row.GetString("mailbox") != identity.GetString("inbox") {
			conflicts = append(conflicts, identity.GetString("email_name")+"@"+domain.GetString("domain"))
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	sort.Strings(conflicts)
	return fmt.Errorf("these addresses already belong to another inbox, and the rule would give them to people: %s. Change those addresses first",
		strings.Join(conflicts, ", "))
}

// registerRuleAddressHooks keeps rule rows current. Every trigger is an
// after-success hook so the change it reacts to is committed.
func registerRuleAddressHooks(app core.App) {
	reconcile := func(a core.App, scope ruleScope) {
		if err := reconcileRuleAddresses(a, scope); err != nil {
			addressLog.Error("rule address reconcile failed", "user", scope.UserID, "domain", scope.DomainID, "error", err)
		}
	}

	app.OnRecordAfterCreateSuccess("mail_identities").BindFunc(func(e *core.RecordEvent) error {
		reconcile(e.App, ruleScope{UserID: e.Record.GetString("user")})
		return e.Next()
	})
	app.OnRecordAfterUpdateSuccess("mail_identities").BindFunc(func(e *core.RecordEvent) error {
		original := e.Record.Original()
		if original.GetString("email_name") != e.Record.GetString("email_name") ||
			original.GetString("inbox") != e.Record.GetString("inbox") {
			reconcile(e.App, ruleScope{UserID: e.Record.GetString("user")})
		}
		return e.Next()
	})

	app.OnRecordUpdate("mail_domains").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.Original().GetString("coverage") != e.Record.GetString("coverage") {
			if err := checkCoverageConflicts(e.App, e.Record); err != nil {
				return err
			}
		}
		return e.Next()
	})
	domainChanged := func(e *core.RecordEvent, becameVerified bool) {
		if becameVerified {
			if err := ensureIdentities(e.App); err != nil {
				addressLog.Error("could not give users identities after a domain was verified", "domain", e.Record.Id, "error", err)
			}
		}
		reconcile(e.App, ruleScope{DomainID: e.Record.Id})
	}
	app.OnRecordAfterCreateSuccess("mail_domains").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetBool("verified") {
			domainChanged(e, true)
		}
		return e.Next()
	})
	app.OnRecordAfterUpdateSuccess("mail_domains").BindFunc(func(e *core.RecordEvent) error {
		original := e.Record.Original()
		becameVerified := !original.GetBool("verified") && e.Record.GetBool("verified")
		if becameVerified || original.GetBool("verified") != e.Record.GetBool("verified") ||
			original.GetString("coverage") != e.Record.GetString("coverage") {
			domainChanged(e, becameVerified)
		}
		return e.Next()
	})

	app.OnRecordAfterUpdateSuccess("users").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.Original().GetString("role") != e.Record.GetString("role") {
			reconcile(e.App, ruleScope{UserID: e.Record.Id})
		}
		return e.Next()
	})
}
