package mail

import (
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/pocketbase/pocketbase/core"
)

// setupFTSMatchApp creates a test app carrying both live-shaped FTS indexes
// (same columns and tokenizer as the shipped migrations) plus seeded body rows.
func setupFTSMatchApp(t *testing.T) core.App {
	t.Helper()
	app := setupInboundTestApp(t)

	for _, ddl := range []string{
		`CREATE VIRTUAL TABLE fts_mail_messages USING fts5(
			record_id UNINDEXED, subject, snippet, sender_name, sender_email, body_text,
			tokenize='` + liveMailFTSTokenizer + `'
		)`,
		`CREATE VIRTUAL TABLE fts_mail_threads USING fts5(
			record_id UNINDEXED, subject, snippet, participants,
			tokenize='` + liveMailFTSTokenizer + `'
		)`,
	} {
		if _, err := app.DB().NewQuery(ddl).Execute(); err != nil {
			t.Fatalf("create fts table: %v", err)
		}
	}

	for id, body := range map[string]string{
		"msg_both":  "alpha beta together",
		"msg_alpha": "alpha only here",
		"msg_beta":  "beta only here",
	} {
		if _, err := app.DB().NewQuery(
			`INSERT INTO fts_mail_messages (record_id, body_text) VALUES ({:id}, {:body})`,
		).Bind(map[string]any{"id": id, "body": body}).Execute(); err != nil {
			t.Fatalf("seed fts row %s: %v", id, err)
		}
	}
	return app
}

func wantExactSet(t *testing.T, got map[string]bool, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("match set = %v, want exactly %v", got, want)
	}
	for _, id := range want {
		if !got[id] {
			t.Fatalf("match set = %v, missing %q", got, id)
		}
	}
}

// RFC 3501: multiple search keys AND. SEARCH BODY "alpha" BODY "beta" must
// return only messages containing BOTH terms — not either.
func TestBuildFTSMatchSet_MultiTermBodyIntersects(t *testing.T) {
	app := setupFTSMatchApp(t)
	s := &imapSession{app: app}

	set := s.buildFTSMatchSet(&imap.SearchCriteria{Body: []string{"alpha", "beta"}})
	wantExactSet(t, set, "msg_both")
}

// Body and Text criteria are separate search keys and likewise AND together.
func TestBuildFTSMatchSet_BodyAndTextIntersect(t *testing.T) {
	app := setupFTSMatchApp(t)
	s := &imapSession{app: app}

	set := s.buildFTSMatchSet(&imap.SearchCriteria{
		Body: []string{"alpha"},
		Text: []string{"beta"},
	})
	wantExactSet(t, set, "msg_both")
}

// A single term still matches everything containing it.
func TestBuildFTSMatchSet_SingleTerm(t *testing.T) {
	app := setupFTSMatchApp(t)
	s := &imapSession{app: app}

	set := s.buildFTSMatchSet(&imap.SearchCriteria{Body: []string{"alpha"}})
	wantExactSet(t, set, "msg_both", "msg_alpha")
}

// Criteria that FTS cannot answer (every term sanitizes away) must fail
// closed — match nothing — rather than silently matching everything.
func TestBuildFTSMatchSet_UnanswerableTermMatchesNothing(t *testing.T) {
	app := setupFTSMatchApp(t)
	s := &imapSession{app: app}

	set := s.buildFTSMatchSet(&imap.SearchCriteria{Body: []string{"\"\""}})
	if set == nil {
		t.Fatal("expected a non-nil (constraining) set for present-but-unanswerable criteria")
	}
	wantExactSet(t, set)
}
