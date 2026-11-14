package mail

import (
	"context"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/addressbook"
)

func TestFactsFor(t *testing.T) {
	addressbook.ResetForTest()
	t.Cleanup(addressbook.ResetForTest)
	app := setupInboundTestApp(t)
	newCorrespondentsCollection(t, app)
	seedCorrespondent(t, app, "u1", "ann@example.com")

	f := factsFor(context.Background(), app, "u1", "Ann@example.com")
	if !f.HasSentTo || f.InContacts != nil {
		t.Fatalf("%+v", f)
	}
	if _, ok := f.evidence()["sender_in_contacts"]; ok {
		t.Fatal("unknown must be absent, not false")
	}

	addressbook.RegisterLookup(func(_ context.Context, _ core.App, _, e string) (bool, error) { return e == "ann@example.com", nil })
	f = factsFor(context.Background(), app, "u1", "ann@example.com")
	if f.InContacts == nil || !*f.InContacts || f.evidence()["sender_in_contacts"] != true {
		t.Fatalf("%+v", f)
	}
	if f.key() == factsFor(context.Background(), app, "u1", "zed@example.com").key() {
		t.Fatal("different facts must group apart")
	}
}

func TestFilterImportantMail_UsesAddressBookEmail(t *testing.T) {
	addressbook.ResetForTest()
	t.Cleanup(addressbook.ResetForTest)
	app := setupInboundTestApp(t)
	addressbook.RegisterLookup(func(_ context.Context, _ core.App, _, e string) (bool, error) { return e == "ann@example.com", nil })
	items := []pendingMail{{sender: "Ann", senderEmail: "ann@example.com"}, {sender: "Zed", senderEmail: "zed@example.com"}}
	got := filterImportantMail(app, "u1", items)
	if len(got) != 1 || got[0].senderEmail != "ann@example.com" {
		t.Fatalf("got %+v", got)
	}
}
