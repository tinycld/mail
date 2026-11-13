package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/automation"
)

func saveTestClassification(t *testing.T, env classifyEnv, userID, label string, preview bool, errText string) *core.Record {
	t.Helper()
	col, err := env.app.FindCollectionByNameOrId("classifications")
	if err != nil {
		t.Fatalf("classifications collection missing: %v", err)
	}
	// One row per (message, user) by unique index, so a later call replaces it.
	if old := rowFor(t, env.app, env.msg.Id, userID); old != nil {
		if err := env.app.Delete(old); err != nil {
			t.Fatalf("failed to replace classification: %v", err)
		}
	}
	rec := core.NewRecord(col)
	rec.Set("app", classifierApp)
	rec.Set("collection", "mail_messages")
	rec.Set("record_id", env.msg.Id)
	rec.Set("user", userID)
	rec.Set("label_name", label)
	rec.Set("preview", preview)
	rec.Set("error", errText)
	rec.Set("action", actionLabelArchive)
	if err := env.app.Save(rec); err != nil {
		t.Fatalf("failed to save classification: %v", err)
	}
	return rec
}

// mustMessage gives the env message a sender, which the helper messages lack.
func mustMessage(t *testing.T, env classifyEnv) *core.Record {
	t.Helper()
	env.msg.Set("sender_email", "s@example.com")
	if err := env.app.Save(env.msg); err != nil {
		t.Fatalf("failed to save message: %v", err)
	}
	return env.msg
}

func hasKey(fields []automation.DynamicField, key string) bool {
	for _, f := range fields {
		if f.Key == key {
			return true
		}
	}
	return false
}

func hasField(fields []automation.DynamicField, key string, value any) bool {
	for _, f := range fields {
		if f.Key == key && f.Value == value {
			return true
		}
	}
	return false
}

func TestClassifiedTrigger_FilterAndOwner(t *testing.T) {
	t.Parallel()
	env := setupClassifyEnv(t)
	row := saveTestClassification(t, env, env.alice, "junk", false, "")
	if !classificationIsMail(env.app, row) {
		t.Fatal("mail row should pass")
	}
	if got := classificationOwner(env.app, row); len(got) != 1 || got[0] != env.alice {
		t.Fatalf("owner = %v", got)
	}
	if classificationIsMail(env.app, saveTestClassification(t, env, env.bob, "junk", true, "")) {
		t.Fatal("preview must not fire")
	}
	if classificationIsMail(env.app, saveTestClassification(t, env, env.bob, "", false, "budget_exceeded")) {
		t.Fatal("error rows must not fire")
	}
}

func TestMessageForRecord_ResolvesClassification(t *testing.T) {
	t.Parallel()
	env := setupClassifyEnv(t)
	row := saveTestClassification(t, env, env.alice, "junk", false, "")
	msg, err := messageForRecord(env.app, row)
	if err != nil || msg.Id != env.msg.Id {
		t.Fatalf("msg=%v err=%v", msg, err)
	}
	same, err := messageForRecord(env.app, env.msg)
	if err != nil || same.Id != env.msg.Id {
		t.Fatalf("message passthrough: %v %v", same, err)
	}
}

func TestActionAudience_ClassifiedOrgRuleActsForRowUserOnly(t *testing.T) {
	t.Parallel()
	env := setupClassifyEnv(t)
	row := saveTestClassification(t, env, env.alice, "junk", false, "")
	rule := newTestRule(t, env.app, env.bob, "org")
	got := actionAudience(env.app, automation.ActionRequest{Rule: rule, OwnerID: env.bob, Record: row})
	if len(got) != 1 || got[0] != env.alice {
		t.Fatalf("audience = %v", got)
	}
}

func TestActionAudience_ClassifiedRowOfNonMemberActsForNobody(t *testing.T) {
	t.Parallel()
	env := setupClassifyEnv(t)
	row := saveTestClassification(t, env, "user_outsider_00", "junk", false, "")
	rule := newTestRule(t, env.app, env.bob, "org")
	if got := actionAudience(env.app, automation.ActionRequest{Rule: rule, OwnerID: env.bob, Record: row}); len(got) != 0 {
		t.Fatalf("audience = %v", got)
	}
}

func TestActionStarMessage_ClassificationRecordStarsRowUsersThread(t *testing.T) {
	t.Parallel()
	env := setupClassifyEnv(t)
	row := saveTestClassification(t, env, env.alice, "junk", false, "")
	rule := newTestRule(t, env.app, env.alice, "personal")
	if err := actionStarMessage(env.app, automation.ActionRequest{Rule: rule, OwnerID: env.alice, Record: row}); err != nil {
		t.Fatalf("actionStarMessage: %v", err)
	}
	if !threadStateFor(t, env.app, env.threadID, env.alice).GetBool("is_starred") {
		t.Error("alice's thread is not starred")
	}
	if threadStateFor(t, env.app, env.threadID, env.bob).GetBool("is_starred") {
		t.Error("bob's thread must stay unstarred")
	}
}

func TestReceivedFields_PersonalOnly(t *testing.T) {
	t.Parallel()
	env := setupClassifyEnv(t)
	seedCorrespondent(t, env.app, env.alice, "s@example.com")
	msg := mustMessage(t, env)
	if f := receivedFields(env.app, "", nil, msg); len(f) != 0 {
		t.Fatal("org rule: no per-user facts")
	}
	f := receivedFields(env.app, env.alice, nil, msg)
	if !hasField(f, "has_sent_to_sender", true) || hasKey(f, "sender_in_contacts") {
		t.Fatalf("fields = %+v", f)
	}
}

func TestClassifiedFields_CarryMessageAndFacts(t *testing.T) {
	t.Parallel()
	env := setupClassifyEnv(t)
	mustMessage(t, env)
	row := saveTestClassification(t, env, env.alice, "junk", false, "")
	f := classifiedFields(env.app, "", nil, row)
	if !hasField(f, "sender_email", "s@example.com") || !hasField(f, "archived", true) ||
		!hasField(f, "has_sent_to_sender", false) || hasKey(f, "sender_in_contacts") {
		t.Fatalf("fields = %+v", f)
	}
}
