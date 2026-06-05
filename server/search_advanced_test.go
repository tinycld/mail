package mail

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The thread FTS index has no body_text column, so its query must carry ONLY
// the sanitized main-query terms — never the body_text-scoped Body (hasWords)
// terms, which would error the whole UNION.
func TestBuildThreadFTSQuery(t *testing.T) {
	t.Run("plain query is sanitized into prefix terms", func(t *testing.T) {
		if got := buildThreadFTSQuery("hello world"); got != `"hello"* "world"*` {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("empty query yields empty (body-only search drops this arm)", func(t *testing.T) {
		if got := buildThreadFTSQuery(""); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})

	t.Run("never references body_text (no such column on this index)", func(t *testing.T) {
		if got := buildThreadFTSQuery("anything"); strings.Contains(got, "body_text") {
			t.Errorf("thread query leaked body_text scoping: %q", got)
		}
	})
}

// The message FTS index has a body_text column; the Body (hasWords) terms are
// scoped to it. The removed "Doesn't have" (notWords) feature must never emit a
// NOT clause.
func TestBuildMessageFTSQuery(t *testing.T) {
	t.Run("plain query is sanitized into prefix terms", func(t *testing.T) {
		if got := buildMessageFTSQuery("hello world", ""); got != `"hello"* "world"*` {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("hasWords terms are scoped to body_text", func(t *testing.T) {
		got := buildMessageFTSQuery("invoice", "refund overdue")
		want := `"invoice"* body_text : "refund"* body_text : "overdue"*`
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("body-only query (no main term) still scopes to body_text", func(t *testing.T) {
		if got := buildMessageFTSQuery("", "refund"); got != `body_text : "refund"*` {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("never emits a NOT clause", func(t *testing.T) {
		for _, c := range []struct{ q, hw string }{
			{"hello", ""}, {"hello", "world"}, {"", "world"}, {"a NOT b", "c"},
		} {
			if got := buildMessageFTSQuery(c.q, c.hw); strings.Contains(got, " NOT ") {
				t.Errorf("buildMessageFTSQuery(%q,%q) leaked a NOT clause: %q", c.q, c.hw, got)
			}
		}
	})

	t.Run("empty inputs yield empty query", func(t *testing.T) {
		if got := buildMessageFTSQuery("", ""); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
}

// ftsUnion includes only the real arms whose FTS query is non-empty, and always
// appends the never-matching placeholder arm so the result is a UNION ALL — that
// keeps snippet()/highlight() legal in the outer aggregate even when only one
// real arm is present (see ftsNoMatchArm / the snippet-context bug it guards).
func TestFTSUnion(t *testing.T) {
	const thr, msg = "/*THREAD_ARM*/", "/*MESSAGE_ARM*/"

	t.Run("both real arms keep both, plus the placeholder", func(t *testing.T) {
		got := ftsUnion(true, thr, true, msg)
		if !strings.Contains(got, thr) || !strings.Contains(got, msg) {
			t.Fatalf("missing a real arm: %q", got)
		}
		// thread + message + placeholder = 3 arms = 2 UNION ALL separators.
		if n := strings.Count(got, "UNION ALL"); n != 2 {
			t.Fatalf("want 2 UNION ALL separators, got %d: %q", n, got)
		}
	})

	t.Run("message-only still UNION ALLs with the placeholder", func(t *testing.T) {
		got := ftsUnion(false, thr, true, msg)
		if strings.Contains(got, thr) {
			t.Fatalf("thread arm should be absent: %q", got)
		}
		if !strings.Contains(got, msg) {
			t.Fatalf("message arm missing: %q", got)
		}
		// message + placeholder = 1 UNION ALL — this is what keeps snippet() legal.
		if n := strings.Count(got, "UNION ALL"); n != 1 {
			t.Fatalf("want 1 UNION ALL (message + placeholder), got %d: %q", n, got)
		}
		if !strings.Contains(got, "WHERE 0") {
			t.Fatalf("placeholder no-match arm missing: %q", got)
		}
	})

	t.Run("thread-only still UNION ALLs with the placeholder", func(t *testing.T) {
		got := ftsUnion(true, thr, false, msg)
		if strings.Contains(got, msg) {
			t.Fatalf("message arm should be absent: %q", got)
		}
		if n := strings.Count(got, "UNION ALL"); n != 1 {
			t.Fatalf("want 1 UNION ALL (thread + placeholder), got %d: %q", n, got)
		}
	})
}

// parseAdvancedFilters must ignore the removed not_words and size_* params even
// if a stale client still sends them — they have no struct fields to land in.
func TestParseAdvancedFiltersIgnoresRemovedParams(t *testing.T) {
	req := httptest.NewRequest(
		"GET",
		"/api/mail/search?from=alice&has_words=invoice&not_words=spam&size_op=gt&size_bytes=1048576&folder=inbox&has_attachment=true",
		nil,
	)
	f := parseAdvancedFilters(req)

	if f.from != "alice" {
		t.Errorf("from = %q, want alice", f.from)
	}
	if f.hasWords != "invoice" {
		t.Errorf("hasWords = %q, want invoice", f.hasWords)
	}
	if f.folder != "inbox" {
		t.Errorf("folder = %q, want inbox", f.folder)
	}
	if !f.hasAttachment {
		t.Error("hasAttachment = false, want true")
	}
	// The removed params are simply dropped — assert the filter set still
	// reports the surviving structured filters correctly.
	if !f.hasStructuredFilters() {
		t.Error("hasStructuredFilters() = false, want true (from/folder/attachment set)")
	}
	if !f.hasAnyFilter() {
		t.Error("hasAnyFilter() = false, want true")
	}
}

// buildMessageWhere must no longer emit a total_size clause.
func TestBuildMessageWhereHasNoSizeClause(t *testing.T) {
	f := advancedFilters{from: "alice", subject: "report", hasAttachment: true}
	params := map[string]any{}
	where := buildMessageWhere(&f, params)

	if strings.Contains(where, "total_size") {
		t.Errorf("WHERE still references total_size: %q", where)
	}
	if _, ok := params["sizeBytes"]; ok {
		t.Error("params still binds sizeBytes")
	}
	// Sanity: the surviving filters are present.
	if !strings.Contains(where, "sender_name LIKE") || !strings.Contains(where, "has_attachments = 1") {
		t.Errorf("expected from + attachment clauses, got %q", where)
	}
}

// An empty filter set produces no WHERE fragment.
func TestBuildMessageWhereEmpty(t *testing.T) {
	f := advancedFilters{}
	if where := buildMessageWhere(&f, map[string]any{}); where != "" {
		t.Errorf("expected empty WHERE, got %q", where)
	}
}
