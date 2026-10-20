package mail

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

type sendAddressEnv struct {
	app            core.App
	inbox          *core.Record
	org, com, test *core.Record
}

func setupSendAddressEnv(t *testing.T) *sendAddressEnv {
	t.Helper()
	app := newAddressTestApp(t)
	org := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.org", "verified": true})
	com := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.com", "verified": true})
	unverified := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "test.example", "verified": false})
	inbox := saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "nathan", "domain": org.Id, "type": "personal", "display_name": "Nathan Stitt", "name": "Nathan",
	})
	saveRealRecord(t, app, "mail_addresses", map[string]any{"local_part": "nathan", "domain": com.Id, "mailbox": inbox.Id, "source": addressSourceManual})
	return &sendAddressEnv{app: app, inbox: inbox, org: org, com: com, test: unverified}
}

func TestSendAddress_ExplicitAddressOnASecondDomain(t *testing.T) {
	t.Parallel()
	env := setupSendAddressEnv(t)
	com, _ := findAddressRow(env.app, "nathan", env.com.Id)

	from, err := resolveSendAddress(env.app, env.inbox, com.Id, "")
	if err != nil {
		t.Fatal(err)
	}
	if from.Email() != "nathan@tinycld.com" || from.Domain.Id != env.com.Id {
		t.Fatalf("sending as %s on domain %s; want nathan@tinycld.com", from.Email(), from.Domain.Id)
	}
}

func TestSendAddress_RefusesAnotherInboxsAddress(t *testing.T) {
	t.Parallel()
	env := setupSendAddressEnv(t)
	saveRealRecord(t, env.app, "mail_mailboxes", map[string]any{
		"address": "support", "domain": env.org.Id, "type": "shared", "display_name": "Support", "name": "Support",
	})
	supportRow, _ := findAddressRow(env.app, "support", env.org.Id)

	_, err := resolveSendAddress(env.app, env.inbox, supportRow.Id, "")
	if !errors.Is(err, errAddressNotInMailbox) {
		t.Fatalf("want errAddressNotInMailbox, got %v", err)
	}
}

// A released client sends alias_id; it must still send from that alias.
func TestSendAddress_LegacyAliasID(t *testing.T) {
	t.Parallel()
	env := setupSendAddressEnv(t)
	alias := saveRealRecord(t, env.app, "mail_mailbox_aliases", map[string]any{"mailbox": env.inbox.Id, "address": "admin"})

	from, err := resolveSendAddress(env.app, env.inbox, "", alias.Id)
	if err != nil {
		t.Fatal(err)
	}
	if from.Email() != "admin@tinycld.org" {
		t.Fatalf("legacy alias sends as %s, want admin@tinycld.org", from.Email())
	}
}

func TestSendAddress_DefaultPrefersVerifiedThenRank(t *testing.T) {
	t.Parallel()
	env := setupSendAddressEnv(t)
	from, err := defaultSendAddress(env.app, env.inbox)
	if err != nil {
		t.Fatal(err)
	}
	if from.Email() != "nathan@tinycld.org" {
		t.Fatalf("default = %s, want the rank-1 verified domain nathan@tinycld.org", from.Email())
	}

	org, err := env.app.FindRecordById("mail_domains", env.org.Id)
	if err != nil {
		t.Fatal(err)
	}
	org.Set("rank", 5)
	if err := env.app.Save(org); err != nil {
		t.Fatal(err)
	}
	from, _ = defaultSendAddress(env.app, env.inbox)
	if from.Email() != "nathan@tinycld.com" {
		t.Fatalf("after reordering, default = %s, want nathan@tinycld.com", from.Email())
	}
}

func TestSendAddress_DefaultFromIdentityWins(t *testing.T) {
	t.Parallel()
	env := setupSendAddressEnv(t)
	user := saveRealRecord(t, env.app, "users", map[string]any{"email": "n@ext.example", "username": "nathan", "name": "Nathan", "role": "owner"})
	com, _ := findAddressRow(env.app, "nathan", env.com.Id)
	saveRealRecord(t, env.app, "mail_identities", map[string]any{"user": user.Id, "inbox": env.inbox.Id, "email_name": "nathan", "default_from": com.Id})

	from, err := defaultSendAddress(env.app, env.inbox)
	if err != nil || from.Email() != "nathan@tinycld.com" {
		t.Fatalf("default = %v, %v; want the identity's default_from", from, err)
	}
}

func TestFormatFromAddress(t *testing.T) {
	t.Parallel()
	if got := formatFromAddress("Nathan Stitt", "nathan@tinycld.com"); got != "Nathan Stitt <nathan@tinycld.com>" {
		t.Fatalf("got %q", got)
	}
	if got := formatFromAddress("", "nathan@tinycld.com"); got != "<nathan@tinycld.com>" {
		t.Fatalf("got %q", got)
	}
}

// An address on a domain that is not verified yet never wins over a verified
// one, even with a better rank.
func TestSendAddress_DefaultSkipsUnverifiedDomainWithBetterRank(t *testing.T) {
	t.Parallel()
	env := setupSendAddressEnv(t)
	env.test.Set("rank", 0)
	if err := env.app.Save(env.test); err != nil {
		t.Fatal(err)
	}
	saveRealRecord(t, env.app, "mail_addresses", map[string]any{"local_part": "nathan", "domain": env.test.Id, "mailbox": env.inbox.Id, "source": addressSourceManual})

	from, err := defaultSendAddress(env.app, env.inbox)
	if err != nil {
		t.Fatal(err)
	}
	if from.Email() != "nathan@tinycld.org" {
		t.Fatalf("default = %s, want the verified nathan@tinycld.org", from.Email())
	}
}

// The inbox moving domains leaves the alias's mirrored row on the old domain;
// a released client's alias_id must still send as that row.
func TestSendAddress_LegacyAliasSurvivesInboxDomainMove(t *testing.T) {
	t.Parallel()
	env := setupSendAddressEnv(t)
	alias := saveRealRecord(t, env.app, "mail_mailbox_aliases", map[string]any{"mailbox": env.inbox.Id, "address": "admin"})
	inbox, err := env.app.FindRecordById("mail_mailboxes", env.inbox.Id)
	if err != nil {
		t.Fatal(err)
	}
	moved := saveRealRecord(t, env.app, "mail_domains", map[string]any{"domain": "moved.example", "verified": true})
	inbox.Set("domain", moved.Id)
	if err := env.app.Save(inbox); err != nil {
		t.Fatal(err)
	}

	from, err := resolveSendAddress(env.app, inbox, "", alias.Id)
	if err != nil {
		t.Fatal(err)
	}
	if from.Email() != "admin@tinycld.org" {
		t.Fatalf("legacy alias sends as %s, want admin@tinycld.org (the mirrored row)", from.Email())
	}
}

// An old client's draft update names no address; it must not overwrite the
// address a newer client chose.
func TestHandleDraft_UpdateWithoutAddressKeepsSentFrom(t *testing.T) {
	t.Parallel()
	env := setupSendAddressEnv(t)
	user := saveRealRecord(t, env.app, "users", map[string]any{"email": "n@ext.example", "username": "nathan", "name": "Nathan", "role": "owner"})
	saveRealRecord(t, env.app, "mail_mailbox_members", map[string]any{"mailbox": env.inbox.Id, "user": user.Id, "role": "owner"})
	com, _ := findAddressRow(env.app, "nathan", env.com.Id)

	re, rec := draftEvent(t, env.app, user, map[string]any{
		"mailbox_id": env.inbox.Id, "from_address_id": com.Id, "subject": "s", "text_body": "one",
	})
	if err := handleDraft(env.app, re); err != nil {
		t.Fatal(err)
	}
	var created struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	re, _ = draftEvent(t, env.app, user, map[string]any{
		"mailbox_id": env.inbox.Id, "message_id": created.MessageID, "subject": "s", "text_body": "two",
	})
	if err := handleDraft(env.app, re); err != nil {
		t.Fatal(err)
	}
	stored, err := env.app.FindRecordById("mail_messages", created.MessageID)
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.GetString("sent_from"); got != com.Id {
		t.Fatalf("sent_from = %q after an address-less update, want %q", got, com.Id)
	}
	if got := stored.GetString("sender_email"); got != "nathan@tinycld.com" {
		t.Fatalf("sender_email = %q, want nathan@tinycld.com", got)
	}
}
