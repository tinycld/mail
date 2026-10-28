package mail

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// An API update of a state row loads the row, then saves every column. When
// an IMAP APPEND marks the thread sent between that load and that save (the
// web app marking the thread read while a mail client saves a reply to
// Sent), the save wrote the stale is_sent=false back, and the reply left the
// Sent folder.
func TestThreadState_APIUpdateKeepsIsSentSetMeanwhile(t *testing.T) {
	t.Parallel()
	var env *addressKeysEnv
	var threadID string
	scenario := tests.ApiScenario{
		Name:            "a member marks a thread read while it is marked sent",
		Method:          http.MethodPatch,
		Body:            strings.NewReader(`{"is_read":true}`),
		Headers:         map[string]string{"Content-Type": "application/json"},
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"is_read":true`},
	}
	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp {
		env = setupAddressKeysEnv(t)
		threadID = env.deliverTo(t, "ann@q.example", "<in-race@sender.example>", "").GetString("thread")
		state := findThreadState(env.app, threadID, env.ann.Id)
		if state == nil {
			t.Fatal("no state row for ann")
		}
		if state.GetBool("is_sent") {
			t.Fatal("precondition: the delivered thread is already marked sent")
		}
		scenario.URL = "/api/collections/mail_thread_state/records/" + state.Id
		scenario.Headers["Authorization"] = authTokenFor(t, env.ann)
		// Runs first: the request has loaded the row, and the APPEND now
		// commits is_sent before the request saves.
		env.app.OnRecordUpdateRequest("mail_thread_state").Bind(&hook.Handler[*core.RecordRequestEvent]{
			Priority: -1,
			Func: func(e *core.RecordRequestEvent) error {
				if err := markThreadSent(e.App, threadID, env.ann.Id); err != nil {
					return err
				}
				return e.Next()
			},
		})
		return env.app
	}
	scenario.AfterTestFunc = func(_ testing.TB, app *tests.TestApp, _ *http.Response) {
		state := findThreadState(app, threadID, env.ann.Id)
		if state == nil || !state.GetBool("is_sent") {
			t.Fatal("the API update wrote is_sent=false over the APPEND's is_sent=true")
		}
		if !state.GetBool("is_read") {
			t.Error("the API update's own is_read was lost")
		}
	}
	scenario.Test(t)
}
