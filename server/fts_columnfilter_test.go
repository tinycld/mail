package mail

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// liveMailFTSTokenizer mirrors the tokenizer used by the live mail FTS
// migrations. Keep in sync — these tests guard that the query builders' output
// actually matches the way mail content is indexed.
const liveMailFTSTokenizer = "porter unicode61"

// newMessageFTS creates an in-memory fts_mail_messages-shaped index (same
// columns as the live migration) so we can assert MATCH behavior without the
// full PocketBase/delivery stack.
func newMessageFTS(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`
		CREATE VIRTUAL TABLE fts_mail_messages USING fts5(
			record_id UNINDEXED, subject, snippet, sender_name, sender_email, body_text,
			tokenize='` + liveMailFTSTokenizer + `'
		)`); err != nil {
		t.Fatalf("create messages fts: %v", err)
	}
	return db
}

// newThreadFTS creates an in-memory fts_mail_threads-shaped index — note it has
// NO body_text column, which is the whole reason buildThreadFTSQuery must never
// emit body_text scoping.
func newThreadFTS(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`
		CREATE VIRTUAL TABLE fts_mail_threads USING fts5(
			record_id UNINDEXED, subject, snippet, participants,
			tokenize='` + liveMailFTSTokenizer + `'
		)`); err != nil {
		t.Fatalf("create threads fts: %v", err)
	}
	return db
}

type msgRow struct {
	id, subject, snippet, senderName, senderEmail, bodyText string
}

func seedMsg(t *testing.T, db *sql.DB, rows ...msgRow) {
	t.Helper()
	for _, r := range rows {
		if _, err := db.Exec(`
			INSERT INTO fts_mail_messages
				(record_id, subject, snippet, sender_name, sender_email, body_text)
			VALUES (?, ?, ?, ?, ?, ?)`,
			r.id, r.subject, r.snippet, r.senderName, r.senderEmail, r.bodyText,
		); err != nil {
			t.Fatalf("seed %q: %v", r.id, err)
		}
	}
}

func msgMatchIDs(t *testing.T, db *sql.DB, ftsQuery string) []string {
	t.Helper()
	if ftsQuery == "" {
		return nil
	}
	rows, err := db.Query(
		`SELECT record_id FROM fts_mail_messages WHERE fts_mail_messages MATCH ? ORDER BY rank`,
		ftsQuery,
	)
	if err != nil {
		t.Fatalf("MATCH %q: %v", ftsQuery, err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func assertMsgMatch(t *testing.T, db *sql.DB, ftsQuery string, want ...string) {
	t.Helper()
	got := msgMatchIDs(t, db, ftsQuery)
	if len(got) != len(want) {
		t.Errorf("query %q: got %v, want %v", ftsQuery, got, want)
		return
	}
	set := map[string]bool{}
	for _, id := range got {
		set[id] = true
	}
	for _, w := range want {
		if !set[w] {
			t.Errorf("query %q: missing %q (got %v)", ftsQuery, w, got)
		}
	}
}

var (
	mInvoice = msgRow{"invoice", "Invoice #42", "snip", "Acme Billing", "billing@acme.com", "Please review the zeppelin refund overdue balance"}
	mWelcome = msgRow{"welcome", "Welcome aboard", "snip", "Bob Jones", "bob@globex.io", "Glad to have you on the team"}
	mReport  = msgRow{"report", "Quarterly report", "snip", "Carol Ng", "carol@initech.dev", "Revenue is up; see the attached zeppelin summary"}
)

// The exact query buildMessageFTSQuery produces for a body-only search must
// match on the body and only the body.
func TestMessageFTSBodyOnlySearch(t *testing.T) {
	db := newMessageFTS(t)
	seedMsg(t, db, mInvoice, mWelcome, mReport)

	// "zeppelin" appears in two bodies (invoice, report) and no subjects.
	assertMsgMatch(t, db, buildMessageFTSQuery("", "zeppelin"), "invoice", "report")
	// A unique body word.
	assertMsgMatch(t, db, buildMessageFTSQuery("", "overdue"), "invoice")
	// Absent term.
	assertMsgMatch(t, db, buildMessageFTSQuery("", "absentxyz"))
}

// body_text scoping must NOT match the same term sitting in another column.
func TestMessageFTSBodyScopeExcludesOtherColumns(t *testing.T) {
	db := newMessageFTS(t)
	// "Welcome" is in the subject, not the body.
	seedMsg(t, db, mWelcome)

	// Plain (unscoped) search finds it via the subject column.
	assertMsgMatch(t, db, buildMessageFTSQuery("welcome", ""), "welcome")
	// Body-scoped search must NOT, because "welcome" isn't in body_text.
	assertMsgMatch(t, db, buildMessageFTSQuery("", "welcome"))
}

// Search-as-you-type: a prefix of a body word matches.
func TestMessageFTSBodyPrefixMatch(t *testing.T) {
	db := newMessageFTS(t)
	seedMsg(t, db, mInvoice, mReport)

	assertMsgMatch(t, db, buildMessageFTSQuery("", "zep"), "invoice", "report")
	assertMsgMatch(t, db, buildMessageFTSQuery("", "overd"), "invoice")
}

// Multiple body terms are AND-ed (all must be present in the body).
func TestMessageFTSMultipleBodyTermsAreAnded(t *testing.T) {
	db := newMessageFTS(t)
	seedMsg(t, db, mInvoice, mReport)

	// "refund" AND "overdue" only co-occur in invoice.
	assertMsgMatch(t, db, buildMessageFTSQuery("", "refund overdue"), "invoice")
	// "zeppelin" AND "summary" only co-occur in report.
	assertMsgMatch(t, db, buildMessageFTSQuery("", "zeppelin summary"), "report")
	// Terms split across two different messages → no single message has both.
	assertMsgMatch(t, db, buildMessageFTSQuery("", "overdue summary"))
}

// Main query + body filter combine: main term matches any column, body term is
// body-scoped, both must hold.
func TestMessageFTSMainQueryPlusBodyFilter(t *testing.T) {
	db := newMessageFTS(t)
	seedMsg(t, db, mInvoice, mReport)

	// main "invoice" (subject of invoice) AND body "zeppelin" → invoice only.
	assertMsgMatch(t, db, buildMessageFTSQuery("invoice", "zeppelin"), "invoice")
	// main "report" (subject of report) AND body "refund" → report has no
	// "refund" in body → no match.
	assertMsgMatch(t, db, buildMessageFTSQuery("report", "refund"))
}

// Matching is case-insensitive (unicode61 folds case).
func TestMessageFTSCaseInsensitive(t *testing.T) {
	db := newMessageFTS(t)
	seedMsg(t, db, mInvoice)

	assertMsgMatch(t, db, buildMessageFTSQuery("", "ZEPPELIN"), "invoice")
	assertMsgMatch(t, db, buildMessageFTSQuery("REVIEW", ""), "invoice")
}

// The thread index has no body_text column; buildThreadFTSQuery must produce a
// query that runs there WITHOUT error (i.e. it never references body_text).
func TestThreadFTSQueryRunsAgainstThreadIndex(t *testing.T) {
	db := newThreadFTS(t)
	if _, err := db.Exec(`
		INSERT INTO fts_mail_threads (record_id, subject, snippet, participants)
		VALUES ('t1', 'Invoice #42', 'snip', 'Acme Billing billing@acme.com')`,
	); err != nil {
		t.Fatalf("seed thread: %v", err)
	}

	var n int
	q := buildThreadFTSQuery("invoice")
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM fts_mail_threads WHERE fts_mail_threads MATCH ?`, q,
	).Scan(&n); err != nil {
		// This is the regression guard: if buildThreadFTSQuery ever leaked a
		// body_text scope, this MATCH would error with "no such column".
		t.Fatalf("thread MATCH %q errored: %v", q, err)
	}
	if n != 1 {
		t.Errorf("thread query %q matched %d, want 1", q, n)
	}
}

// Reproduce the outer-query shape the handler uses (FTS subquery feeding an
// outer GROUP BY with snippet()/highlight()) for a MESSAGE-ONLY search, and
// assert the ftsUnion placeholder arm keeps snippet() legal. Without the
// placeholder this errors with "unable to use function snippet in the requested
// context" — the exact bug that made body-only search return 0 results.
func TestMessageOnlyUnionAllowsSnippet(t *testing.T) {
	db := newMessageFTS(t)
	seedMsg(t, db, mInvoice)

	messageArm := `
		SELECT fts_mail_messages.record_id as thread_id,
			   snippet(fts_mail_messages, 5, '<', '>', '...', 40) as snippet_highlight,
			   fts_mail_messages.rank as rank
		FROM fts_mail_messages
		WHERE fts_mail_messages MATCH ?`

	// The placeholder must match the 3-column shape used in this test.
	const testNoMatchArm = `SELECT '' as thread_id, '' as snippet_highlight, 0.0 as rank WHERE 0`

	run := func(t *testing.T, body string) ([]string, error) {
		t.Helper()
		sql := `SELECT thread_id, MAX(snippet_highlight) as sh FROM (` +
			body + `) GROUP BY thread_id`
		rows, err := db.Query(sql, buildMessageFTSQuery("", "zeppelin"))
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var id, sh string
			if err := rows.Scan(&id, &sh); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	}

	t.Run("sole FTS arm under GROUP BY errors on snippet()", func(t *testing.T) {
		// Characterize the underlying SQLite limitation this guards against.
		if _, err := run(t, messageArm); err == nil {
			t.Skip("this SQLite build allows snippet() on a sole arm; placeholder not strictly needed")
		}
	})

	t.Run("UNION ALL with placeholder makes snippet() legal", func(t *testing.T) {
		ids, err := run(t, messageArm+"\nUNION ALL\n"+testNoMatchArm)
		if err != nil {
			t.Fatalf("snippet() should be legal with the placeholder arm, got: %v", err)
		}
		if len(ids) != 1 || ids[0] != "invoice" {
			t.Fatalf("want [invoice], got %v", ids)
		}
	})
}

// Demonstrate WHY the split exists: running a body_text-scoped query against the
// thread index errors. This is exactly the bug that returned 0 results before
// the split. If FTS5 ever stopped erroring here, the split could be revisited.
func TestThreadIndexRejectsBodyTextScope(t *testing.T) {
	db := newThreadFTS(t)
	if _, err := db.Exec(
		`INSERT INTO fts_mail_threads (record_id, subject, snippet, participants) VALUES ('t1','S','x','y')`,
	); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// The OLD combined query shape: body_text scoping sent at the thread index.
	bad := buildMessageFTSQuery("", "zeppelin") // contains `body_text : ...`
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM fts_mail_threads WHERE fts_mail_threads MATCH ?`, bad,
	).Scan(&n)
	if err == nil {
		t.Fatalf("expected an FTS error for body_text scope on thread index, got %d rows", n)
	}
}
