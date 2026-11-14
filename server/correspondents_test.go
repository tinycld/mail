package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// newCorrespondentsCollection mirrors migration 2040000019, with user as plain
// text so tests need no users collection.
func newCorrespondentsCollection(t *testing.T, app core.App) {
	t.Helper()
	col := core.NewBaseCollection("mail_correspondents")
	col.Fields.Add(&core.TextField{Name: "user", Required: true})
	col.Fields.Add(&core.TextField{Name: "email", Required: true, Max: 320})
	col.Fields.Add(&core.DateField{Name: "last_sent"})
	col.AddIndex("idx_mail_correspondents_user_email", true, "user, email", "")
	if err := app.Save(col); err != nil {
		t.Fatalf("failed to save mail_correspondents: %v", err)
	}
}

func seedCorrespondent(t *testing.T, app core.App, userID, email string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("mail_correspondents")
	if err != nil {
		t.Fatalf("mail_correspondents collection missing: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("user", userID)
	rec.Set("email", email)
	if err := app.Save(rec); err != nil {
		t.Fatalf("failed to seed correspondent: %v", err)
	}
}

func TestRecordCorrespondents_SentMessage(t *testing.T) {
	app := setupInboundTestApp(t)
	newCorrespondentsCollection(t, app)
	seedDomainAndMailbox(t, app, "corr-one.test", "alice", "mb_corr_000001")
	thread := newTestThread(t, app, padID("mb_corr_000001"), "s")
	msg := newTestMessage(t, app, thread.Id, "s")
	msg.Set("sent_by", "user_alice_0000")
	msg.Set("delivery_status", "sent")
	msg.Set("recipients_to", `[{"name":"Ann","email":"Ann@Example.com"}]`)
	msg.Set("recipients_cc", `[{"email":"cc@example.com"}]`)
	if err := recordCorrespondents(app, msg); err != nil {
		t.Fatal(err)
	}
	if !hasSentTo(app, "user_alice_0000", "ann@example.com") || !hasSentTo(app, "user_alice_0000", "CC@example.com") {
		t.Fatal("recipients not recorded")
	}
	if hasSentTo(app, "user_bob_000000", "ann@example.com") {
		t.Fatal("other user must not match")
	}
	if err := recordCorrespondents(app, msg); err != nil {
		t.Fatal("second call must be idempotent:", err)
	}
}

func TestRecordCorrespondentsOnUpdate_OnlyWhenBecomingSent(t *testing.T) {
	app := setupInboundTestApp(t)
	newCorrespondentsCollection(t, app)
	seedDomainAndMailbox(t, app, "corr-three.test", "alice", "mb_corr_000003")
	thread := newTestThread(t, app, padID("mb_corr_000003"), "s")
	msg := newTestMessage(t, app, thread.Id, "s")
	msg.Set("sent_by", "user_alice_0000")
	msg.Set("delivery_status", "draft")
	msg.Set("recipients_to", `[{"email":"later@example.com"}]`)
	if err := app.Save(msg); err != nil {
		t.Fatal(err)
	}

	loaded, err := app.FindRecordById("mail_messages", msg.Id)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Set("delivery_status", "sent")
	if err := recordCorrespondentsOnUpdate(app, loaded); err != nil {
		t.Fatal(err)
	}
	if !hasSentTo(app, "user_alice_0000", "later@example.com") {
		t.Fatal("a draft that became sent must record its recipients")
	}

	if err := app.Save(loaded); err != nil {
		t.Fatal(err)
	}
	deleted, err := app.FindFirstRecordByFilter("mail_correspondents", "email = 'later@example.com'")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Delete(deleted); err != nil {
		t.Fatal(err)
	}
	flagged, err := app.FindRecordById("mail_messages", msg.Id)
	if err != nil {
		t.Fatal(err)
	}
	flagged.Set("is_starred", true)
	if err := recordCorrespondentsOnUpdate(app, flagged); err != nil {
		t.Fatal(err)
	}
	if hasSentTo(app, "user_alice_0000", "later@example.com") {
		t.Fatal("a flag update on an already-sent message must write nothing")
	}
}

func TestRecordCorrespondents_IgnoresInboundAndDrafts(t *testing.T) {
	app := setupInboundTestApp(t)
	newCorrespondentsCollection(t, app)
	seedDomainAndMailbox(t, app, "corr-two.test", "alice", "mb_corr_000002")
	thread := newTestThread(t, app, padID("mb_corr_000002"), "s")
	msg := newTestMessage(t, app, thread.Id, "s")
	msg.Set("recipients_to", `[{"email":"x@example.com"}]`)
	_ = recordCorrespondents(app, msg)
	msg.Set("sent_by", "user_alice_0000")
	msg.Set("delivery_status", "draft")
	_ = recordCorrespondents(app, msg)
	if hasSentTo(app, "user_alice_0000", "x@example.com") {
		t.Fatal("inbound/draft must not record")
	}
}
