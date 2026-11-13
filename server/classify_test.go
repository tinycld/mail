package mail

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/classifier"
	"tinycld.org/core/classifier/classifiertest"
	"tinycld.org/core/classifier/fakeprovider"
	"tinycld.org/core/syscfg"
)

type classifyEnv struct {
	actionEnv
	fake        *fakeprovider.Fake
	junkLabelID string
}

// installClassifier builds every collection the classification job touches
// and wires a fake provider, so tests run the real classifier end to end.
func installClassifier(t *testing.T, app core.App, settings map[string]string) (*fakeprovider.Fake, string) {
	t.Helper()
	classifiertest.CreateCollections(t, app)
	newCorrespondentsCollection(t, app)

	assignments := core.NewBaseCollection("label_assignments")
	for _, name := range []string{"label", "record_id", "collection", "user"} {
		assignments.Fields.Add(&core.TextField{Name: name})
	}
	mustSave(t, app, assignments)

	for _, name := range []string{"user_preferences", "settings"} {
		col := core.NewBaseCollection(name)
		col.Fields.Add(&core.TextField{Name: "app"})
		col.Fields.Add(&core.TextField{Name: "key"})
		col.Fields.Add(&core.JSONField{Name: "value"})
		col.Fields.Add(&core.TextField{Name: "user"})
		mustSave(t, app, col)
	}

	seedOrgLabel(t, app, "personal", "A message from a person to me.", false, 0)
	junkLabel := seedOrgLabel(t, app, "junk", "Unsolicited promotion or spam.", true, 1)

	fake := fakeprovider.New()
	classifier.SetProvider(app, fake)
	syscfg.SetResolver(app, func(key string) string { return settings[key] })
	return fake, junkLabel
}

func mustSave(t *testing.T, app core.App, m core.Model) {
	t.Helper()
	if err := app.Save(m); err != nil {
		t.Fatal(err)
	}
}

func seedClassifierRow(t *testing.T, app core.App, userID, name, when string, auto bool, order int) string {
	t.Helper()
	labelsCol, _ := app.FindCollectionByNameOrId("labels")
	label := core.NewRecord(labelsCol)
	label.Set("name", name)
	label.Set("user", userID)
	mustSave(t, app, label)
	taxCol, _ := app.FindCollectionByNameOrId("classifier_labels")
	row := core.NewRecord(taxCol)
	row.Set("app", classifierApp)
	row.Set("label", label.Id)
	row.Set("assign_when", when)
	row.Set("auto_action", auto)
	row.Set("order", order)
	row.Set("user", userID)
	mustSave(t, app, row)
	return label.Id
}

func seedOrgLabel(t *testing.T, app core.App, name, when string, auto bool, order int) string {
	return seedClassifierRow(t, app, "", name, when, auto, order)
}

func seedPersonalTaxonomy(t *testing.T, app core.App, userID, name string) {
	t.Helper()
	seedClassifierRow(t, app, userID, name, "Newsletters I subscribed to.", false, 0)
	seedClassifierRow(t, app, userID, "other", "Anything else.", false, 1)
}

func withSetting(key, value string) func(map[string]string) {
	return func(m map[string]string) { m[key] = value }
}

func setupClassifyEnv(t *testing.T, opts ...func(map[string]string)) classifyEnv {
	t.Helper()
	env := setupActionEnv(t, "classify.test", "mb_classify_0001")
	settings := map[string]string{classifier.KeyAPIKey: fakeprovider.AcceptedKey}
	for _, o := range opts {
		o(settings)
	}
	fake, junkID := installClassifier(t, env.app, settings)
	_ = ensureThreadState(env.app, env.threadID, env.alice, "inbox", false)
	_ = ensureThreadState(env.app, env.threadID, env.bob, "inbox", false)
	setThreadMessageCount(t, env.app, env.threadID, 1)
	return classifyEnv{actionEnv: env, fake: fake, junkLabelID: junkID}
}

func (e classifyEnv) input(subject string) classifyInput {
	return classifyInput{
		MessageID: e.msg.Id, ThreadID: e.threadID, MailboxID: e.mailboxID,
		Msg: &InboundMessage{From: Recipient{Email: "s@example.com"}, Subject: subject, TextBody: "body"},
	}
}

func setPref(t *testing.T, app core.App, userID, key, value string) {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("user_preferences")
	rec := core.NewRecord(col)
	raw, _ := json.Marshal(value)
	rec.Set("user", userID)
	rec.Set("app", classifierApp)
	rec.Set("key", key)
	rec.Set("value", json.RawMessage(raw))
	mustSave(t, app, rec)
}

func rowFor(t *testing.T, app core.App, messageID, userID string) *core.Record {
	t.Helper()
	rec, err := app.FindFirstRecordByFilter("classifications", "record_id = {:r} && user = {:u}",
		map[string]any{"r": messageID, "u": userID})
	if err != nil {
		return nil
	}
	return rec
}

func assertRow(t *testing.T, app core.App, messageID, userID, label, action string) {
	t.Helper()
	rec := rowFor(t, app, messageID, userID)
	if rec == nil {
		t.Fatalf("no classification row for %s", userID)
	}
	if rec.GetString("label_name") != label || rec.GetString("action") != action {
		t.Fatalf("row for %s = %s/%s, want %s/%s (error %q)", userID,
			rec.GetString("label_name"), rec.GetString("action"), label, action, rec.GetString("error"))
	}
}

func assertNoRow(t *testing.T, app core.App, messageID, userID string) {
	t.Helper()
	if rowFor(t, app, messageID, userID) != nil {
		t.Fatalf("unexpected classification row for %s", userID)
	}
}

func labelAssignments(t *testing.T, app core.App, threadID, userID, labelID string) int {
	t.Helper()
	state := threadStateFor(t, app, threadID, userID)
	recs, err := app.FindRecordsByFilter("label_assignments",
		"label = {:l} && record_id = {:r} && user = {:u}", "", 0, 0,
		map[string]any{"l": labelID, "r": state.Id, "u": userID})
	if err != nil {
		t.Fatal(err)
	}
	return len(recs)
}

func assertLabelled(t *testing.T, app core.App, threadID, userID, labelID string) {
	t.Helper()
	if labelAssignments(t, app, threadID, userID, labelID) != 1 {
		t.Fatalf("thread not labelled for %s", userID)
	}
}

func assertNotLabelled(t *testing.T, app core.App, threadID, userID string) {
	t.Helper()
	recs, _ := app.FindRecordsByFilter("label_assignments", "user = {:u}", "", 0, 0, map[string]any{"u": userID})
	if len(recs) != 0 {
		t.Fatalf("thread unexpectedly labelled for %s", userID)
	}
}

func TestClassifyMessage_SharedMailboxMixedModes(t *testing.T) {
	env := setupClassifyEnv(t)
	setPref(t, env.app, env.alice, "classifier.mode", "labels_archive")
	setPref(t, env.app, env.bob, "classifier.mode", "labels")

	if err := classifyMessage(context.Background(), env.app, env.input("[jev:junk] Buy now")); err != nil {
		t.Fatal(err)
	}
	if env.fake.Calls.Load() != 1 {
		t.Fatalf("one taxonomy + same facts → one call, got %d", env.fake.Calls.Load())
	}
	if threadStateFor(t, env.app, env.threadID, env.alice).GetString("folder") != "archive" {
		t.Fatal("alice should be archived")
	}
	if threadStateFor(t, env.app, env.threadID, env.bob).GetString("folder") != "inbox" {
		t.Fatal("bob should stay in inbox")
	}
	assertLabelled(t, env.app, env.threadID, env.bob, env.junkLabelID)
	assertRow(t, env.app, env.messageID(), env.alice, "junk", "label_archive")
	assertRow(t, env.app, env.messageID(), env.bob, "junk", "label")
}

func (e classifyEnv) messageID() string { return e.msg.Id }

func (e classifyEnv) classify(t *testing.T, subject string) {
	t.Helper()
	if err := classifyMessage(context.Background(), e.app, e.input(subject)); err != nil {
		t.Fatal(err)
	}
}

func TestClassifyMessage_OffAndPreview(t *testing.T) {
	env := setupClassifyEnv(t)
	setPref(t, env.app, env.alice, "classifier.mode", "preview")
	// bob has no pref → off
	env.classify(t, "[jev:junk] Buy now")
	assertRow(t, env.app, env.messageID(), env.alice, "junk", "none")
	assertNotLabelled(t, env.app, env.threadID, env.alice)
	assertNoRow(t, env.app, env.messageID(), env.bob)
	recs, _ := env.app.FindRecordsByFilter("classifications", "user = {:u}", "", 0, 0, map[string]any{"u": env.alice})
	if !recs[0].GetBool("preview") {
		t.Fatal("preview flag")
	}
}

func TestClassifyMessage_PersonalTaxonomySeparateCall(t *testing.T) {
	env := setupClassifyEnv(t)
	setPref(t, env.app, env.alice, "classifier.mode", "labels")
	setPref(t, env.app, env.bob, "classifier.mode", "labels")
	setPref(t, env.app, env.bob, "classifier.taxonomy", "personal")
	seedPersonalTaxonomy(t, env.app, env.bob, "newsletters")
	env.classify(t, "[jev:newsletters] x")
	if env.fake.Calls.Load() != 2 {
		t.Fatalf("calls = %d", env.fake.Calls.Load())
	}
	assertRow(t, env.app, env.messageID(), env.bob, "newsletters", "label")
	// The org taxonomy has no "newsletters", so the fake gets no match for
	// alice's group and the call fails; her row proves she used the org set.
	if rec := rowFor(t, env.app, env.messageID(), env.alice); rec == nil || rec.GetString("label_name") == "newsletters" {
		t.Fatal("alice must classify against the org taxonomy")
	}
}

func TestClassifyMessage_PersonalWithoutRowsFallsBackToOrg(t *testing.T) {
	env := setupClassifyEnv(t)
	setPref(t, env.app, env.bob, "classifier.mode", "labels")
	setPref(t, env.app, env.bob, "classifier.taxonomy", "personal")
	env.classify(t, "[jev:junk] x")
	assertRow(t, env.app, env.messageID(), env.bob, "junk", "label")
	assertLabelled(t, env.app, env.threadID, env.bob, env.junkLabelID)
}

func TestClassifyMessage_ArchiveFailureRecordsLabelOnly(t *testing.T) {
	env := setupClassifyEnv(t)
	setPref(t, env.app, env.alice, "classifier.mode", "labels_archive")
	env.app.OnRecordUpdate("mail_thread_state").BindFunc(func(e *core.RecordEvent) error {
		return errors.New("archive blocked")
	})
	env.classify(t, "[jev:junk] Buy now")
	assertLabelled(t, env.app, env.threadID, env.alice, env.junkLabelID)
	assertRow(t, env.app, env.messageID(), env.alice, "junk", "label")
}

func TestClassifyMessage_BudgetExceededRecordsErrorOnly(t *testing.T) {
	env := setupClassifyEnv(t, withSetting("classifier.monthly_budget_usd", "0"))
	setPref(t, env.app, env.alice, "classifier.mode", "labels")
	env.classify(t, "[jev:junk] x")
	recs, _ := env.app.FindRecordsByFilter("classifications", "user = {:u}", "", 0, 0, map[string]any{"u": env.alice})
	if len(recs) != 1 || recs[0].GetString("error") != "budget_exceeded" || env.fake.Calls.Load() != 0 {
		t.Fatalf("recs=%d calls=%d", len(recs), env.fake.Calls.Load())
	}
	assertNotLabelled(t, env.app, env.threadID, env.alice)
}

func TestClassifyMessage_NoKeyDoesNothing(t *testing.T) {
	env := setupClassifyEnv(t, withSetting("classifier.api_key", ""))
	setPref(t, env.app, env.alice, "classifier.mode", "labels")
	env.classify(t, "[jev:junk] x")
	assertNoRow(t, env.app, env.messageID(), env.alice)
}

func TestArchiveThreshold_FloorAndCeiling(t *testing.T) {
	cases := map[string]float64{
		"0.4": 0.9, "0.5": 0.5, "0.75": 0.75, "1": 1, "1.1": 0.9, "abc": 0.9, "": 0.9,
	}
	for in, want := range cases {
		env := setupClassifyEnv(t)
		col, _ := env.app.FindCollectionByNameOrId("settings")
		rec := core.NewRecord(col)
		raw, _ := json.Marshal(in)
		rec.Set("app", classifierApp)
		rec.Set("key", settingArchiveThreshold)
		rec.Set("value", json.RawMessage(raw))
		mustSave(t, env.app, rec)
		if got := archiveThreshold(env.app); got != want {
			t.Errorf("threshold %q = %v, want %v", in, got, want)
		}
	}
}
