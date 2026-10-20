package mail

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// The thread updateRule lets any member write mail_threads, and the server
// copies some thread columns to every member's state row. The columns the
// server derives from the messages are kept as stored on an API update, so
// one member cannot change them for the whole inbox. subject stays writable.
func TestThreadOwnedFields_MemberCannotUpdate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		field string
		value string
	}{
		{"address_keys", `"zzz"`},
		{"latest_date", `"2030-01-01 00:00:00.000Z"`},
		{"has_draft", `true`},
		{"has_attachments", `true`},
		{"message_count", `99`},
		{"snippet", `"edited snippet"`},
		{"participants", `[{"name":"X","email":"x@x.example"}]`},
		{"mailbox", `"OTHER"`},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			var env *addressKeysEnv
			var threadID, before, stateDateBefore string
			scenario := tests.ApiScenario{
				Name:            "a member PATCHes " + tc.field + " on a thread",
				Method:          http.MethodPatch,
				Headers:         map[string]string{"Content-Type": "application/json"},
				ExpectedStatus:  http.StatusOK,
				ExpectedContent: []string{`"subject":"edited"`},
			}
			scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp {
				env = setupAddressKeysEnv(t)
				// A second inbox ann belongs to, so the rule allows a move
				// and only the guard keeps the thread where it is.
				other := saveRealRecord(t, env.app, "mail_mailboxes", map[string]any{
					"address": "other", "domain": env.desk.GetString("domain"), "type": "shared",
					"display_name": "Other", "name": "Other",
				})
				saveRealRecord(t, env.app, "mail_mailbox_members", map[string]any{"mailbox": other.Id, "user": env.ann.Id, "role": "owner"})
				thread := mustRecord(t, env.app, "mail_threads",
					env.deliverTo(t, "ann@q.example", "<owned-1@sender.example>", "").GetString("thread"))
				threadID = thread.Id
				before = thread.GetString(tc.field)
				stateDateBefore = findThreadState(env.app, threadID, env.ann.Id).GetString("latest_date")
				value := strings.Replace(tc.value, "OTHER", other.Id, 1)
				scenario.Body = strings.NewReader(`{"subject":"edited","` + tc.field + `":` + value + `}`)
				scenario.URL = "/api/collections/mail_threads/records/" + threadID
				scenario.Headers["Authorization"] = authTokenFor(t, env.ann)
				return env.app
			}
			scenario.AfterTestFunc = func(_ testing.TB, app *tests.TestApp, _ *http.Response) {
				thread := mustRecord(t, app, "mail_threads", threadID)
				if got := thread.GetString(tc.field); got != before {
					t.Errorf("%s = %s after the PATCH, want the stored %s", tc.field, got, before)
				}
				if got := findThreadState(app, threadID, env.ann.Id).GetString("latest_date"); got != stateDateBefore {
					t.Errorf("ann's state latest_date = %q after the PATCH, want %q", got, stateDateBefore)
				}
			}
			scenario.Test(t)
		})
	}
}

// On create the columns the messages decide start empty; the messages that
// follow set them. The others are kept, because the seed creates threads
// through the API with them.
func TestThreadOwnedFields_Create(t *testing.T) {
	t.Parallel()
	scenario := tests.ApiScenario{
		Name:           "a member creates a thread with server-owned columns",
		Method:         http.MethodPost,
		URL:            "/api/collections/mail_threads/records",
		Headers:        map[string]string{"Content-Type": "application/json"},
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"address_keys":""`, `"has_draft":false`, `"has_attachments":false`,
			`"latest_date":"2026-10-01 09:00:00.000Z"`, `"snippet":"hello"`, `"message_count":2`,
		},
	}
	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp {
		env := setupAddressKeysEnv(t)
		scenario.Body = strings.NewReader(`{"mailbox":"` + env.desk.Id + `","subject":"New","address_keys":"zzz",` +
			`"has_draft":true,"has_attachments":true,"latest_date":"2026-10-01 09:00:00.000Z",` +
			`"snippet":"hello","message_count":2}`)
		scenario.Headers["Authorization"] = authTokenFor(t, env.ann)
		return env.app
	}
	scenario.Test(t)
}
