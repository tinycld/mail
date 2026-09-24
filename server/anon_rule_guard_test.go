package mail

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/rlstest"
)

// anon_rule_guard_test.go proves that a caller with no login cannot reach a
// mailbox that has no members, against the SHIPPED migrations (rlstest).
//
// With no login, PocketBase resolves @request.auth.id to NULL and compiles
// `x = NULL` to `(x = '' OR x IS NULL)`. A back-relation such as
// `mail_mailbox_members_via_mailbox.user ?= @request.auth.id` is a LEFT JOIN,
// so a mailbox with ZERO member rows yields NULL and matches the anonymous
// caller. A shared mailbox becomes memberless when its last member deletes
// their account (lifecycle.go sweeps only personal mailboxes). Before
// 1830000007 an anonymous caller could then list, view, edit, create and
// delete that mailbox's threads and messages.

const loginGuard = `@request.auth.id != "" && `

type anonGuardEnv struct {
	*mailShareEnv
	memberThread  *core.Record
	memberMessage *core.Record
	orphanMailbox *core.Record
	orphanThread  *core.Record
	orphanMessage *core.Record
}

func setupAnonGuardApp(t *testing.T) *anonGuardEnv {
	t.Helper()
	env := setupMailShareApp(t)

	domain, err := env.app.FindRecordById("mail_domains", env.mailbox.GetString("domain"))
	if err != nil {
		t.Fatal(err)
	}
	mailboxes, err := env.app.FindCollectionByNameOrId("mail_mailboxes")
	if err != nil {
		t.Fatal(err)
	}
	orphan := core.NewRecord(mailboxes)
	orphan.Set("address", "sales")
	orphan.Set("name", "Sales")
	orphan.Set("domain", domain.Id)
	orphan.Set("type", "shared")
	if err := env.app.Save(orphan); err != nil {
		t.Fatalf("save orphan mailbox: %v", err)
	}

	memberThread, memberMessage := saveThreadWithMessage(t, env.app, env.mailbox, "member thread")
	orphanThread, orphanMessage := saveThreadWithMessage(t, env.app, orphan, "sales secrets")

	return &anonGuardEnv{
		mailShareEnv:  env,
		memberThread:  memberThread,
		memberMessage: memberMessage,
		orphanMailbox: orphan,
		orphanThread:  orphanThread,
		orphanMessage: orphanMessage,
	}
}

func saveThreadWithMessage(t *testing.T, app core.App, mailbox *core.Record, subject string) (*core.Record, *core.Record) {
	t.Helper()
	threads, err := app.FindCollectionByNameOrId("mail_threads")
	if err != nil {
		t.Fatal(err)
	}
	thread := core.NewRecord(threads)
	thread.Set("mailbox", mailbox.Id)
	thread.Set("subject", subject)
	if err := app.Save(thread); err != nil {
		t.Fatalf("save thread: %v", err)
	}

	messages, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatal(err)
	}
	message := core.NewRecord(messages)
	message.Set("thread", thread.Id)
	message.Set("sender_email", "sender@example.test")
	message.Set("date", "2026-09-24 10:00:00.000Z")
	message.Set("subject", subject)
	if err := app.Save(message); err != nil {
		t.Fatalf("save message: %v", err)
	}
	return thread, message
}

type anonCall struct {
	method   string
	url      string
	body     string
	status   int
	expected []string
	absent   []string
}

// run sends the call with the given token; an empty token sends no
// Authorization header at all, which is the anonymous caller.
func (c anonCall) run(t *testing.T, env *anonGuardEnv, token string) {
	t.Helper()
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = token
	}
	scenario := &tests.ApiScenario{
		Name:                  c.method + " " + c.url,
		Method:                c.method,
		URL:                   c.url,
		Headers:               headers,
		ExpectedStatus:        c.status,
		ExpectedContent:       c.expected,
		NotExpectedContent:    c.absent,
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return env.app },
		DisableTestAppCleanup: true,
	}
	if c.body != "" {
		scenario.Body = strings.NewReader(c.body)
		scenario.Headers["Content-Type"] = "application/json"
	}
	if c.status != http.StatusNoContent && len(c.expected) == 0 {
		scenario.ExpectedContent = []string{`"message"`}
	}
	scenario.Test(t)
}

func recordURL(collection, id string) string {
	return "/api/collections/" + collection + "/records/" + id
}

func listURL(collection string) string {
	return "/api/collections/" + collection + "/records"
}

func TestAnonGuard_ListMailboxesHidesMemberlessMailbox(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodGet, url: listURL("mail_mailboxes"), status: http.StatusOK,
		expected: []string{`"totalItems":0`}, absent: []string{env.orphanMailbox.Id},
	}.run(t, env, "")
}

func TestAnonGuard_ViewMemberlessMailboxDenied(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodGet, url: recordURL("mail_mailboxes", env.orphanMailbox.Id),
		status: http.StatusNotFound,
	}.run(t, env, "")
}

func TestAnonGuard_UpdateMemberlessMailboxDenied(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodPatch, url: recordURL("mail_mailboxes", env.orphanMailbox.Id),
		body: `{"name":"pwned"}`, status: http.StatusNotFound,
	}.run(t, env, "")
}

func TestAnonGuard_DeleteMemberlessMailboxDenied(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodDelete, url: recordURL("mail_mailboxes", env.orphanMailbox.Id),
		status: http.StatusNotFound,
	}.run(t, env, "")
}

func TestAnonGuard_ListThreadsHidesMemberlessMailbox(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodGet, url: listURL("mail_threads"), status: http.StatusOK,
		expected: []string{`"totalItems":0`}, absent: []string{"sales secrets"},
	}.run(t, env, "")
}

func TestAnonGuard_ViewMemberlessThreadDenied(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodGet, url: recordURL("mail_threads", env.orphanThread.Id),
		status: http.StatusNotFound,
	}.run(t, env, "")
}

func TestAnonGuard_UpdateMemberlessThreadDenied(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodPatch, url: recordURL("mail_threads", env.orphanThread.Id),
		body: `{"subject":"pwned"}`, status: http.StatusNotFound,
	}.run(t, env, "")
}

func TestAnonGuard_DeleteMemberlessThreadDenied(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodDelete, url: recordURL("mail_threads", env.orphanThread.Id),
		status: http.StatusNotFound,
	}.run(t, env, "")
}

func TestAnonGuard_CreateThreadInMemberlessMailboxDenied(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodPost, url: listURL("mail_threads"),
		body:   `{"mailbox":"` + env.orphanMailbox.Id + `","subject":"planted"}`,
		status: http.StatusBadRequest,
	}.run(t, env, "")
}

func TestAnonGuard_ListMessagesHidesMemberlessMailbox(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodGet, url: listURL("mail_messages"), status: http.StatusOK,
		expected: []string{`"totalItems":0`}, absent: []string{env.orphanMessage.Id},
	}.run(t, env, "")
}

func TestAnonGuard_ViewMemberlessMessageDenied(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodGet, url: recordURL("mail_messages", env.orphanMessage.Id),
		status: http.StatusNotFound,
	}.run(t, env, "")
}

func TestAnonGuard_UpdateMemberlessMessageDenied(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodPatch, url: recordURL("mail_messages", env.orphanMessage.Id),
		body: `{"subject":"pwned"}`, status: http.StatusNotFound,
	}.run(t, env, "")
}

func TestAnonGuard_DeleteMemberlessMessageDenied(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodDelete, url: recordURL("mail_messages", env.orphanMessage.Id),
		status: http.StatusNotFound,
	}.run(t, env, "")
}

func TestAnonGuard_CreateMessageInMemberlessThreadDenied(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodPost, url: listURL("mail_messages"),
		body: `{"thread":"` + env.orphanThread.Id + `","sender_email":"x@example.test",` +
			`"date":"2026-09-24 10:00:00.000Z","subject":"planted"}`,
		status: http.StatusBadRequest,
	}.run(t, env, "")
}

// Positive control: the guard must not lock out a logged-in member. Each
// case also confirms the member does not see the memberless mailbox.
func TestAnonGuard_MemberKeepsAccess(t *testing.T) {
	cases := []struct {
		name string
		call func(env *anonGuardEnv) anonCall
	}{
		{"list mailboxes", func(env *anonGuardEnv) anonCall {
			return anonCall{
				method: http.MethodGet, url: listURL("mail_mailboxes"), status: http.StatusOK,
				expected: []string{`"totalItems":1`, env.mailbox.Id}, absent: []string{env.orphanMailbox.Id},
			}
		}},
		{"view mailbox", func(env *anonGuardEnv) anonCall {
			return anonCall{
				method: http.MethodGet, url: recordURL("mail_mailboxes", env.mailbox.Id),
				status: http.StatusOK, expected: []string{env.mailbox.Id},
			}
		}},
		{"update mailbox", func(env *anonGuardEnv) anonCall {
			return anonCall{
				method: http.MethodPatch, url: recordURL("mail_mailboxes", env.mailbox.Id),
				body: `{"name":"Renamed"}`, status: http.StatusOK, expected: []string{`"name":"Renamed"`},
			}
		}},
		{"delete mailbox", func(env *anonGuardEnv) anonCall {
			return anonCall{
				method: http.MethodDelete, url: recordURL("mail_mailboxes", env.mailbox.Id),
				status: http.StatusNoContent,
			}
		}},
		{"list threads", func(env *anonGuardEnv) anonCall {
			return anonCall{
				method: http.MethodGet, url: listURL("mail_threads"), status: http.StatusOK,
				expected: []string{`"totalItems":1`, "member thread"}, absent: []string{"sales secrets"},
			}
		}},
		{"view thread", func(env *anonGuardEnv) anonCall {
			return anonCall{
				method: http.MethodGet, url: recordURL("mail_threads", env.memberThread.Id),
				status: http.StatusOK, expected: []string{env.memberThread.Id},
			}
		}},
		{"update thread", func(env *anonGuardEnv) anonCall {
			return anonCall{
				method: http.MethodPatch, url: recordURL("mail_threads", env.memberThread.Id),
				body: `{"subject":"edited"}`, status: http.StatusOK, expected: []string{`"subject":"edited"`},
			}
		}},
		{"delete thread", func(env *anonGuardEnv) anonCall {
			return anonCall{
				method: http.MethodDelete, url: recordURL("mail_threads", env.memberThread.Id),
				status: http.StatusNoContent,
			}
		}},
		{"list messages", func(env *anonGuardEnv) anonCall {
			return anonCall{
				method: http.MethodGet, url: listURL("mail_messages"), status: http.StatusOK,
				expected: []string{`"totalItems":1`, env.memberMessage.Id}, absent: []string{env.orphanMessage.Id},
			}
		}},
		{"view message", func(env *anonGuardEnv) anonCall {
			return anonCall{
				method: http.MethodGet, url: recordURL("mail_messages", env.memberMessage.Id),
				status: http.StatusOK, expected: []string{env.memberMessage.Id},
			}
		}},
		{"update message", func(env *anonGuardEnv) anonCall {
			return anonCall{
				method: http.MethodPatch, url: recordURL("mail_messages", env.memberMessage.Id),
				body: `{"subject":"edited"}`, status: http.StatusOK, expected: []string{`"subject":"edited"`},
			}
		}},
		{"delete message", func(env *anonGuardEnv) anonCall {
			return anonCall{
				method: http.MethodDelete, url: recordURL("mail_messages", env.memberMessage.Id),
				status: http.StatusNoContent,
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := setupAnonGuardApp(t)
			tc.call(env).run(t, env, env.ownerToken)
		})
	}
}

// Shipped-rule literal check. Every mail rule that reads @request.auth must
// open with the login guard: a later migration that restates a rule and
// drops it reopens the memberless-mailbox hole, and this names the rule.
// Negative-only checks (`@request.auth.role != "guest"`,
// `@request.auth.disabled != true`) are true for an anonymous caller, so they
// do not count as a guard.
func TestAnonGuard_ShippedRulesRequireLogin(t *testing.T) {
	env := setupMailShareApp(t)
	cols, err := env.app.FindAllCollections()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, col := range cols {
		if !strings.HasPrefix(col.Name, "mail_") {
			continue
		}
		for _, kind := range []string{"list", "view", "create", "update", "delete"} {
			rule, ok := rlstest.Rule(t, env.app, col.Name, kind)
			if !ok || !strings.Contains(rule, "@request.auth") {
				continue
			}
			checked++
			if !strings.HasPrefix(rule, loginGuard) {
				t.Errorf("%s.%sRule must start with %q\n  shipped rule: %s",
					col.Name, kind, loginGuard, rule)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no mail rule reads @request.auth; the collection scan found nothing to check")
	}
}

// Names the rules this fix guards, so a dropped collection or verb cannot
// pass the scan above by no longer being scanned.
func TestAnonGuard_ShippedMembershipRulesCarryGuard(t *testing.T) {
	env := setupMailShareApp(t)
	all := []string{"list", "view", "create", "update", "delete"}
	guarded := map[string][]string{
		"mail_mailboxes":       all,
		"mail_mailbox_members": all,
		"mail_threads":         all,
		"mail_messages":        all,
		"mail_thread_state":    all,
		"mail_folder_counts":   {"list", "view"},
	}
	for collection, kinds := range guarded {
		for _, kind := range kinds {
			rlstest.RequireRuleContains(t, env.app, collection, kind, loginGuard)
		}
	}
}
