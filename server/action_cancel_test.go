package mail

import (
	"context"
	"errors"
	"testing"

	"tinycld.org/core/automation"
	"tinycld.org/core/search"
)

// The engine cancels an action's context when the action times out and when
// the app terminates; the database can close right after. A handler given an
// ended context must not write or send.

func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestThreadStateActions_CancelledContextWritesNothing(t *testing.T) {
	actions := map[string]automation.ActionHandler{
		"move-to-folder": actionMoveToFolder,
		"mark-as-read":   actionMarkAsRead,
		"star-message":   actionStarMessage,
	}
	for name, action := range actions {
		t.Run(name, func(t *testing.T) {
			env := setupActionEnv(t, "cancel-"+name+".test", "mb_cancel_"+name)
			rule := newTestRule(t, env.app, env.alice, "org")

			err := action(env.app, automation.ActionRequest{
				Rule: rule, OwnerID: env.alice, Record: env.msg,
				Params: map[string]string{"folder": "archive"},
				Ctx:    cancelledContext(),
			})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
			for _, userID := range []string{env.alice, env.bob} {
				if state := findThreadState(env.app, env.threadID, userID); state != nil {
					t.Errorf("user %s got a thread state row after the context ended", userID)
				}
			}
		})
	}
}

func TestRuleSends_CancelledContextSendsNothing(t *testing.T) {
	e := seedLoopEnv(t)
	postmarkInbound(t, e.app, inboundPayload("ann@q.example", "Question", "<cancel-1@sender.example>"))
	message := e.received(t, e.ann, "Question")

	sends := map[string]automation.ActionHandler{
		"send-message":    actionSendMessage,
		"forward-message": actionForwardMessage,
	}
	for name, action := range sends {
		t.Run(name, func(t *testing.T) {
			err := action(e.app, automation.ActionRequest{
				Rule: e.rule(t, e.ann), OwnerID: e.ann.Id, Record: message,
				Params: map[string]string{"to": "sender@sender.example", "subject": "Got it", "body": "Thanks"},
				Ctx:    cancelledContext(),
			})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
			requireSends(t, e.sent, 0)
		})
	}
}

func TestSearchMailRows_CancelledContextRunsNoQuery(t *testing.T) {
	app := setupFTSMatchApp(t)
	seedDomainAndMailbox(t, app, "acme.com", "alice", "mb_searchcancel1")
	user := searchUser(t, app, "alice@acme.com")
	seedMember(t, app, "mb_searchcancel1", user.Id)

	res, err := searchMailRows(app, user.Id, search.Query{
		Include: []string{"alpha"},
		Limit:   25,
		Ctx:     cancelledContext(),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(res.Rows) != 0 {
		t.Fatalf("rows = %v, want none", res.Rows)
	}
}
