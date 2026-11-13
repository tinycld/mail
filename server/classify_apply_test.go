package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/classifier"
)

func junk(conf float64, mismatch bool) classifier.Result {
	return classifier.Result{Label: classifier.TaxonomyLabel{Name: "junk", AutoAction: true}, Confidence: conf, ArgmaxMismatch: mismatch}
}

func setThreadMessageCount(t *testing.T, app core.App, threadID string, n int) {
	t.Helper()
	thread, err := app.FindRecordById("mail_threads", threadID)
	if err != nil {
		t.Fatal(err)
	}
	thread.Set("message_count", n)
	if err := app.Save(thread); err != nil {
		t.Fatal(err)
	}
}

func TestDecideAction(t *testing.T) {
	env := setupActionEnv(t, "decide.test", "mb_decide_00001")
	_ = ensureThreadState(env.app, env.threadID, env.alice, "inbox", false)
	setThreadMessageCount(t, env.app, env.threadID, 1)

	cases := []struct {
		name, mode string
		r          classifier.Result
		want       string
	}{
		{"preview", modePreview, junk(0.99, false), "none"},
		{"labels mode", modeLabels, junk(0.99, false), "label"},
		{"archive", modeLabelsArchive, junk(0.95, false), "label_archive"},
		{"below threshold", modeLabelsArchive, junk(0.85, false), "label"},
		{"argmax mismatch", modeLabelsArchive, junk(0.99, true), "label"},
		{"not auto", modeLabelsArchive, classifier.Result{Label: classifier.TaxonomyLabel{Name: "personal"}, Confidence: 0.99}, "label"},
	}
	for _, c := range cases {
		if got := decideAction(env.app, env.alice, c.mode, env.threadID, c.r, 0.9); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func TestDecideAction_NeverArchivesOngoingOrMovedThreads(t *testing.T) {
	env := setupActionEnv(t, "decide2.test", "mb_decide_00002")
	_ = ensureThreadState(env.app, env.threadID, env.alice, "inbox", false)
	setThreadMessageCount(t, env.app, env.threadID, 2)
	if got := decideAction(env.app, env.alice, modeLabelsArchive, env.threadID, junk(0.99, false), 0.9); got != "label" {
		t.Fatalf("ongoing thread: %s", got)
	}
	setThreadMessageCount(t, env.app, env.threadID, 1)
	_ = setThreadFolder(env.app, env.threadID, env.alice, "spam")
	if got := decideAction(env.app, env.alice, modeLabelsArchive, env.threadID, junk(0.99, false), 0.9); got != "label" {
		t.Fatalf("moved thread: %s", got)
	}
}
