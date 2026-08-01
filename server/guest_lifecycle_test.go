package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// guest_lifecycle_test.go proves the users-create hook does not auto-provision
// mail infrastructure for share-link guests.
//
// Core's invite flow creates guests as real `users` rows, and the users-create
// hook fires for every row. Without a role check, a guest silently receives a
// working <username>@<verified-domain> address plus an owner membership — which
// passes verifyMailboxMembership, grants IMAP login, and receives inbound mail.
// That contradicts the guest-exclusion rules (1830000002/1830000003), which
// exist to keep guests out of mail infra entirely.

// countMailboxMemberships returns how many membership rows point at the user.
func countMailboxMemberships(t *testing.T, app core.App, userID string) int {
	t.Helper()
	rows, err := app.FindRecordsByFilter(
		"mail_mailbox_members", "user = {:user}", "", 0, 0,
		map[string]any{"user": userID},
	)
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

func TestHandleUserCreated_GuestGetsNoMailbox(t *testing.T) {
	env := setupMailGuestApp(t)

	// The bare test users collection has no username field; derivation needs it.
	users, err := env.app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	users.Fields.Add(&core.TextField{Name: "username"})
	if err := env.app.Save(users); err != nil {
		t.Fatal(err)
	}

	// The bare test mail_domains collection has no `created` autodate, which
	// the hook sorts by when picking the first verified domain.
	domains, err := env.app.FindCollectionByNameOrId("mail_domains")
	if err != nil {
		t.Fatal(err)
	}
	domains.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	if err := env.app.Save(domains); err != nil {
		t.Fatal(err)
	}
	setUsername := func(rec *core.Record, username string) *core.Record {
		fresh, err := env.app.FindRecordById("users", rec.Id)
		if err != nil {
			t.Fatal(err)
		}
		fresh.Set("username", username)
		if err := env.app.Save(fresh); err != nil {
			t.Fatal(err)
		}
		return fresh
	}
	member := setUsername(env.member, "alice")
	guest := setUsername(env.guest, "eve")

	// Positive control: a real member is provisioned, so the guest assertion
	// below fails only on the role check and not on a broken fixture.
	handleUserCreated(env.app, member)
	if n := countMailboxMemberships(t, env.app, member.Id); n != 1 {
		t.Fatalf("member should have 1 mailbox membership, got %d", n)
	}

	handleUserCreated(env.app, guest)
	if n := countMailboxMemberships(t, env.app, guest.Id); n != 0 {
		t.Fatalf("guest must have no mailbox membership, got %d", n)
	}
	if mbs, err := env.app.FindRecordsByFilter(
		"mail_mailboxes", "address = 'eve'", "", 0, 0, nil,
	); err == nil && len(mbs) > 0 {
		t.Fatal("guest must not receive a mailbox address")
	}
}
