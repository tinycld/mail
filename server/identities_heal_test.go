package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

type healFixture struct {
	app   core.App
	main  *core.Record
	other *core.Record
	user  *core.Record
	inbox *core.Record
	ident *core.Record
}

// healSetup gives one person an identity whose inbox sits on a non-main domain.
func healSetup(t *testing.T) healFixture {
	t.Helper()
	app := newRuleTestApp(t)
	main := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "a.example", "verified": true, "rank": 1})
	other := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "b.example", "verified": true, "rank": 2})
	user := saveRealRecord(t, app, "users", map[string]any{"email": "dana@ext.example", "username": "dana", "name": "Dana", "role": "member"})
	// The user hook already made an identity and inbox on the main domain;
	// move the person onto the other domain.
	auto := identityOf(t, app, user.Id)
	autoInbox := auto.GetString("inbox")
	if err := app.Delete(auto); err != nil {
		t.Fatal(err)
	}
	autoBox, err := app.FindRecordById("mail_mailboxes", autoInbox)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Delete(autoBox); err != nil {
		t.Fatal(err)
	}
	inbox := saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "dana", "domain": other.Id, "type": "personal", "display_name": "Dana", "name": "Dana"})
	saveRealRecord(t, app, "mail_mailbox_members", map[string]any{"mailbox": inbox.Id, "user": user.Id, "role": "owner"})
	ident := saveRealRecord(t, app, "mail_identities", map[string]any{"user": user.Id, "inbox": inbox.Id, "email_name": "dana"})
	return healFixture{app, main, other, user, inbox, ident}
}

func requireHealed(t *testing.T, f healFixture, oldInbox string) {
	t.Helper()
	identity := identityOf(t, f.app, f.user.Id)
	newInbox := identity.GetString("inbox")
	if newInbox == "" || newInbox == oldInbox {
		t.Fatalf("identity inbox = %q, want a new inbox", newInbox)
	}
	inbox, err := f.app.FindRecordById("mail_mailboxes", newInbox)
	if err != nil || inbox.GetString("domain") != f.main.Id || inbox.GetString("type") != "personal" {
		t.Fatalf("new inbox = %v, %v; want a personal inbox on the main domain", inbox, err)
	}
	if row, _ := findAddressRow(f.app, "dana", f.main.Id); row == nil || row.GetString("mailbox") != newInbox {
		t.Fatal("the new inbox has no primary address dana@main")
	}
}

func TestHeal_RemovingTheInboxDomainGivesANewInbox(t *testing.T) {
	t.Parallel()
	f := healSetup(t)
	third := saveRealRecord(t, f.app, "mail_domains", map[string]any{"domain": "c.example", "verified": true, "rank": 3, "coverage": coverageEveryone})
	if err := f.app.Delete(reloaded(t, f.app, f.other)); err != nil {
		t.Fatal(err)
	}
	requireHealed(t, f, f.inbox.Id)
	newInbox := identityOf(t, f.app, f.user.Id).GetString("inbox")
	if got := ruleRows(t, f.app, third.Id); got["dana"] != newInbox {
		t.Fatalf("rule rows on the other everyone domain = %v, want dana -> %s", got, newInbox)
	}
}

func TestHeal_OwnerDeletingTheirInbox(t *testing.T) {
	t.Parallel()
	for name, policy := range map[string]inboxPolicy{"boot": adoptInboxOnly, "create": adoptOrCreateInbox} {
		t.Run(name, func(t *testing.T) {
			f := healSetup(t)
			if err := f.app.Delete(reloaded(t, f.app, f.inbox)); err != nil {
				t.Fatal(err)
			}
			if identityOf(t, f.app, f.user.Id).GetString("inbox") != "" {
				t.Fatal("precondition: deleting the inbox clears identity.inbox")
			}
			if _, err := ensureIdentityWith(f.app, reloaded(t, f.app, f.user), policy); err != nil {
				t.Fatal(err)
			}
			requireHealed(t, f, f.inbox.Id)
		})
	}
}

func TestHeal_AdoptsAnUnclaimedPersonalInbox(t *testing.T) {
	t.Parallel()
	f := healSetup(t)
	spare := saveRealRecord(t, f.app, "mail_mailboxes", map[string]any{"address": "dana2", "domain": f.other.Id, "type": "personal", "display_name": "Dana", "name": "Dana"})
	saveRealRecord(t, f.app, "mail_mailbox_members", map[string]any{"mailbox": spare.Id, "user": f.user.Id, "role": "owner"})
	if err := f.app.Delete(reloaded(t, f.app, f.inbox)); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureIdentity(f.app, reloaded(t, f.app, f.user)); err != nil {
		t.Fatal(err)
	}
	if got := identityOf(t, f.app, f.user.Id).GetString("inbox"); got != spare.Id {
		t.Fatalf("inbox = %q, want the unclaimed inbox %q", got, spare.Id)
	}
}

func TestHeal_TakenEmailNameIsReplaced(t *testing.T) {
	t.Parallel()
	f := healSetup(t)
	if err := f.app.Delete(reloaded(t, f.app, f.inbox)); err != nil {
		t.Fatal(err)
	}
	saveRealRecord(t, f.app, "mail_mailboxes", map[string]any{"address": "dana", "domain": f.main.Id, "type": "shared", "display_name": "Desk", "name": "Desk"})
	if _, err := ensureIdentity(f.app, reloaded(t, f.app, f.user)); err != nil {
		t.Fatal(err)
	}
	identity := identityOf(t, f.app, f.user.Id)
	if identity.GetString("inbox") == "" || identity.GetString("email_name") != "dana2" {
		t.Fatalf("identity inbox %q name %q, want a new inbox named dana2", identity.GetString("inbox"), identity.GetString("email_name"))
	}
}

func TestHeal_RetiredIdentityIsNotRepaired(t *testing.T) {
	t.Parallel()
	f := healSetup(t)
	if err := retireIdentity(f.app, f.user.Id); err != nil {
		t.Fatal(err)
	}
	for _, policy := range []inboxPolicy{adoptInboxOnly, adoptOrCreateInbox} {
		if got, err := ensureIdentityWith(f.app, reloaded(t, f.app, f.user), policy); err != nil || got != nil {
			t.Fatalf("ensureIdentityWith = %v, %v; want nil", got, err)
		}
	}
	if err := repairEmptyInboxIdentities(f.app); err != nil {
		t.Fatal(err)
	}
	if identityOf(t, f.app, f.user.Id).GetString("inbox") != "" {
		t.Fatal("a retired identity got an inbox")
	}
}

func TestHeal_DomainDeleteCreatesNoIdentityForUsersWithout(t *testing.T) {
	t.Parallel()
	f := healSetup(t)
	bare := saveRealRecord(t, f.app, "users", map[string]any{"email": "bo@ext.example", "username": "bo", "name": "Bo", "role": "member"})
	auto := identityOf(t, f.app, bare.Id)
	autoInbox := auto.GetString("inbox")
	if err := f.app.Delete(auto); err != nil {
		t.Fatal(err)
	}
	box, err := f.app.FindRecordById("mail_mailboxes", autoInbox)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.app.Delete(box); err != nil {
		t.Fatal(err)
	}
	if err := f.app.Delete(reloaded(t, f.app, f.other)); err != nil {
		t.Fatal(err)
	}
	if identityOf(t, f.app, bare.Id) != nil {
		t.Fatal("a domain delete created an identity for a user without one")
	}
	if err := ensureIdentities(f.app, adoptInboxOnly); err != nil {
		t.Fatal(err)
	}
	if identityOf(t, f.app, bare.Id) != nil {
		t.Fatal("boot created an identity for a user without an inbox")
	}
}
