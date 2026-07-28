package mail

import (
	"fmt"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// seedMailbox creates a bare mailbox row so derivation sees the address taken.
func seedMailbox(t *testing.T, app core.App, address, domainID string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("mail_mailboxes")
	if err != nil {
		t.Fatal(err)
	}
	mb := core.NewRecord(col)
	mb.Set("address", address)
	mb.Set("domain", domainID)
	mb.Set("type", "personal")
	if err := app.Save(mb); err != nil {
		t.Fatalf("seed mailbox %s: %v", address, err)
	}
}

func TestDeriveMailboxAddress_UsesUsernameVerbatim(t *testing.T) {
	app := setupAliasTestApp(t)
	domainID := seedDomainAndMailbox(t, app, "example.com", "taken", "mbox_derive_01")

	if got := deriveMailboxAddress(app, "alice", domainID); got != "alice" {
		t.Fatalf("deriveMailboxAddress(alice) = %q, want alice", got)
	}
	// Legacy rows may predate username validation; case and spacing normalize.
	if got := deriveMailboxAddress(app, " Alice ", domainID); got != "alice" {
		t.Fatalf("deriveMailboxAddress(' Alice ') = %q, want alice", got)
	}
}

func TestDeriveMailboxAddress_SuffixesOnCollision(t *testing.T) {
	app := setupAliasTestApp(t)
	domainID := seedDomainAndMailbox(t, app, "example.com", "bob", "mbox_derive_02")

	if got := deriveMailboxAddress(app, "bob", domainID); got != "bob2" {
		t.Fatalf("first collision = %q, want bob2", got)
	}
	seedMailbox(t, app, "bob2", domainID)
	if got := deriveMailboxAddress(app, "bob", domainID); got != "bob3" {
		t.Fatalf("second collision = %q, want bob3", got)
	}
}

// A username that sanitizes to nothing (all-unicode legacy row) yields no
// address. handleUserCreated turns that into "create no mailbox" with an
// Error log — a deliberate trade: account creation must not fail over mailbox
// provisioning, but the outcome must be loud enough to notice.
func TestDeriveMailboxAddress_UnsanitizableUsernameYieldsNoAddress(t *testing.T) {
	app := setupAliasTestApp(t)
	domainID := seedDomainAndMailbox(t, app, "example.com", "taken", "mbox_derive_03")

	if got := deriveMailboxAddress(app, "日本語", domainID); got != "" {
		t.Fatalf("deriveMailboxAddress(unicode) = %q, want empty", got)
	}
}

// The suffix loop caps at 99. The 100th "bob" gets no address at all — same
// loud-but-non-fatal contract as above.
func TestDeriveMailboxAddress_SuffixExhaustionYieldsNoAddress(t *testing.T) {
	app := setupAliasTestApp(t)
	domainID := seedDomainAndMailbox(t, app, "example.com", "bob", "mbox_derive_04")
	for i := 2; i <= 99; i++ {
		seedMailbox(t, app, fmt.Sprintf("bob%d", i), domainID)
	}

	if got := deriveMailboxAddress(app, "bob", domainID); got != "" {
		t.Fatalf("exhausted derivation = %q, want empty", got)
	}
}

// handleUserDeleted must sweep only memberless PERSONAL mailboxes: one that
// still has a member stays (its owner is alive), and a memberless shared
// mailbox is out of scope (shared mailboxes are administered, not owned).
func TestHandleUserDeleted_SweepsOnlyMemberlessPersonalMailboxes(t *testing.T) {
	app := setupInboundTestApp(t)

	domainID := seedDomainAndMailbox(t, app, "example.com", "kept", "mbox_sweep_keep")
	seedMember(t, app, "mbox_sweep_keep", "user_alive_00001")

	// Memberless personal mailbox: the one the sweep exists to remove.
	seedMailbox(t, app, "orphan", domainID)

	// Memberless SHARED mailbox: must survive the sweep.
	mailboxesCol, err := app.FindCollectionByNameOrId("mail_mailboxes")
	if err != nil {
		t.Fatal(err)
	}
	shared := core.NewRecord(mailboxesCol)
	shared.Set("address", "team")
	shared.Set("domain", domainID)
	shared.Set("type", "shared")
	if err := app.Save(shared); err != nil {
		t.Fatal(err)
	}

	handleUserDeleted(app, nil)

	if _, err := app.FindFirstRecordByFilter("mail_mailboxes", "address = 'kept'"); err != nil {
		t.Fatalf("membered personal mailbox was swept: %v", err)
	}
	if _, err := app.FindFirstRecordByFilter("mail_mailboxes", "address = 'orphan'"); err == nil {
		t.Fatal("memberless personal mailbox survived the sweep")
	}
	if _, err := app.FindFirstRecordByFilter("mail_mailboxes", "address = 'team'"); err != nil {
		t.Fatalf("shared mailbox was swept: %v", err)
	}
}
