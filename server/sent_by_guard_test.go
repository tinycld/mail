package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupSentByGuardApp builds the smallest schema the guard touches: a
// mail_messages collection with sent_by, and two users to attribute between.
func setupSentByGuardApp(t *testing.T) (*tests.TestApp, *core.Record, *core.Record) {
	t.Helper()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	messages := core.NewBaseCollection("mail_messages")
	messages.Fields.Add(&core.TextField{Name: "subject"})
	messages.Fields.Add(&core.RelationField{
		Name: "sent_by", CollectionId: users.Id, MaxSelect: 1,
	})
	if err := app.Save(messages); err != nil {
		t.Fatalf("save mail_messages: %v", err)
	}

	mkUser := func(email string) *core.Record {
		u := core.NewRecord(users)
		u.Set("email", email)
		u.SetPassword("Password123!")
		if err := app.Save(u); err != nil {
			t.Fatalf("save user %s: %v", email, err)
		}
		return u
	}

	return app, mkUser("spammer@example.com"), mkUser("colleague@example.com")
}

// newRequestEvent builds a RecordRequestEvent the way the guard sees one, with
// the given record and authenticated caller.
func newRequestEvent(t *testing.T, app *tests.TestApp, record, auth *core.Record) *core.RecordRequestEvent {
	t.Helper()

	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatal(err)
	}
	e := &core.RecordRequestEvent{
		RequestEvent: &core.RequestEvent{Auth: auth, App: app},
		Record:       record,
	}
	// Tags() reads the embedded collection; without it the tagged hook filter
	// skips our handler and the test passes for the wrong reason.
	e.Collection = messages
	return e
}

// The point of the field is to survive a member who wants their sending
// blamed on somebody else. mail_messages' update rule is mailbox membership,
// so without the guard any member can PATCH sent_by to another user's id.
func TestSentByGuard_UpdateCannotReassignTheSender(t *testing.T) {
	app, spammer, colleague := setupSentByGuardApp(t)
	registerSentByGuard(app)

	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatal(err)
	}
	msg := core.NewRecord(messages)
	msg.Set("subject", "buy now")
	msg.Set("sent_by", spammer.Id)
	if err := app.Save(msg); err != nil {
		t.Fatalf("seed message: %v", err)
	}

	// A real PATCH loads the stored row, so Original() is populated. Mutating
	// the same in-memory record we just saved would leave Original() empty and
	// the guard would restore "" — passing for the wrong reason.
	fresh, err := app.FindRecordById("mail_messages", msg.Id)
	if err != nil {
		t.Fatal(err)
	}

	// The spammer PATCHes the row, pointing sent_by at a colleague.
	fresh.Set("sent_by", colleague.Id)
	if err := app.OnRecordUpdateRequest("mail_messages").Trigger(
		newRequestEvent(t, app, fresh, spammer),
		func(e *core.RecordRequestEvent) error { return nil },
	); err != nil {
		t.Fatalf("trigger update hook: %v", err)
	}

	if got := fresh.GetString("sent_by"); got != spammer.Id {
		t.Errorf("sent_by = %q, want it pinned to the real sender %q", got, spammer.Id)
	}
}

// A client-created row never went through the send path, so there is no
// sender to name and an attacker must not be able to invent one.
func TestSentByGuard_CreateCannotClaimASender(t *testing.T) {
	app, spammer, colleague := setupSentByGuardApp(t)
	registerSentByGuard(app)

	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatal(err)
	}
	msg := core.NewRecord(messages)
	msg.Set("subject", "forged")
	msg.Set("sent_by", colleague.Id)

	if err := app.OnRecordCreateRequest("mail_messages").Trigger(
		newRequestEvent(t, app, msg, spammer),
		func(e *core.RecordRequestEvent) error { return nil },
	); err != nil {
		t.Fatalf("trigger create hook: %v", err)
	}

	if got := msg.GetString("sent_by"); got != "" {
		t.Errorf("sent_by = %q, want empty — a client-created row has no sender", got)
	}
}

// An ordinary mailbox operation sends the whole record back, including
// sent_by. The guard must restore the field rather than refuse the request,
// or filing and flagging mail breaks.
func TestSentByGuard_UnrelatedUpdateStillSucceeds(t *testing.T) {
	app, spammer, _ := setupSentByGuardApp(t)
	registerSentByGuard(app)

	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatal(err)
	}
	msg := core.NewRecord(messages)
	msg.Set("subject", "original")
	msg.Set("sent_by", spammer.Id)
	if err := app.Save(msg); err != nil {
		t.Fatalf("seed message: %v", err)
	}

	fresh, err := app.FindRecordById("mail_messages", msg.Id)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Set("subject", "refiled")
	nextCalled := false
	if err := app.OnRecordUpdateRequest("mail_messages").Trigger(
		newRequestEvent(t, app, fresh, spammer),
		func(e *core.RecordRequestEvent) error { nextCalled = true; return nil },
	); err != nil {
		t.Fatalf("trigger update hook: %v", err)
	}

	if !nextCalled {
		t.Error("the guard must not refuse an ordinary update")
	}
	if got := fresh.GetString("subject"); got != "refiled" {
		t.Errorf("subject = %q, want the caller's change to stick", got)
	}
	if got := fresh.GetString("sent_by"); got != spammer.Id {
		t.Errorf("sent_by = %q, want it preserved", got)
	}
}

// storeMessage writes sent_by through app.Save, which does not fire the
// ...Request hooks. If that ever changes, the send paths silently stop
// recording attribution and every test above still passes.
func TestSentByGuard_ServerSideSaveStillRecordsTheSender(t *testing.T) {
	app, spammer, _ := setupSentByGuardApp(t)
	registerSentByGuard(app)

	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatal(err)
	}
	msg := core.NewRecord(messages)
	msg.Set("subject", "a real send")
	msg.Set("sent_by", spammer.Id)
	if err := app.Save(msg); err != nil {
		t.Fatalf("save: %v", err)
	}

	stored, err := app.FindRecordById("mail_messages", msg.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.GetString("sent_by"); got != spammer.Id {
		t.Errorf("sent_by = %q, want %q — a server-side save must still attribute", got, spammer.Id)
	}
}
