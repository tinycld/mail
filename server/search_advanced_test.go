package mail

import (
	"net/http/httptest"
	"strings"
	"testing"

	"tinycld.org/packages/mail/api"
)

// The thread FTS index has no body_text column, so its query must carry ONLY
// the sanitized main-query terms — never the body_text-scoped Body (hasWords)
// terms, which would error the whole UNION.
func TestBuildThreadFTSQuery(t *testing.T) {
	t.Run("plain query is sanitized into prefix terms", func(t *testing.T) {
		if got := buildThreadFTSQuery("hello world", ""); got != `"hello"* "world"*` {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("empty query yields empty (body-only search drops this arm)", func(t *testing.T) {
		if got := buildThreadFTSQuery("", ""); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})

	t.Run("never references body_text (no such column on this index)", func(t *testing.T) {
		if got := buildThreadFTSQuery("anything", ""); strings.Contains(got, "body_text") {
			t.Errorf("thread query leaked body_text scoping: %q", got)
		}
	})
}

// The message FTS index has a body_text column; the Body (hasWords) terms are
// scoped to it.
func TestBuildMessageFTSQuery(t *testing.T) {
	t.Run("plain query is sanitized into prefix terms", func(t *testing.T) {
		if got := buildMessageFTSQuery("hello world", "", ""); got != `"hello"* "world"*` {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("hasWords terms are scoped to body_text", func(t *testing.T) {
		got := buildMessageFTSQuery("invoice", "refund overdue", "")
		want := `"invoice"* body_text : "refund"* body_text : "overdue"*`
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})

	t.Run("body-only query (no main term) still scopes to body_text", func(t *testing.T) {
		if got := buildMessageFTSQuery("", "refund", ""); got != `body_text : "refund"*` {
			t.Fatalf("got %q", got)
		}
	})

	// Originally this subtest asserted a blanket prohibition: buildMessageFTSQuery
	// must NEVER emit a NOT clause, full stop. That was correct when the function
	// had no exclude parameter — the "doesn't have" filter was removed in commit
	// dc988fd (2026-06-04) because "FTS5 NOT-only queries error without a positive
	// term", and this subtest was the regression guard against reintroducing it
	// carelessly.
	//
	// Exclusion is back, but gated: appendExclusions only emits NOT when `base`
	// (the sanitized positive-term query) is non-empty. That gate — not a
	// blanket ban — is the real invariant, so this subtest is narrowed to assert
	// the gate instead of being deleted: no positive term still means no NOT
	// (the original cases, unchanged, now with an exclude term present too, to
	// prove the gate blocks it even when the caller asks for exclusion), and a
	// positive term now legitimately produces NOT clauses.
	t.Run("NOT clause requires a positive term (the gate)", func(t *testing.T) {
		t.Run("no positive term: NOT is suppressed even with an exclude term", func(t *testing.T) {
			// hasWords also counts as a positive term (it becomes a body_text-scoped
			// clause in `base`), so these cases hold both q and hw empty — the only
			// way `base` is truly empty.
			for _, c := range []struct{ q, hw, exclude string }{
				{"", "", ""}, {"", "", "spam"},
			} {
				if got := buildMessageFTSQuery(c.q, c.hw, c.exclude); strings.Contains(got, " NOT ") {
					t.Errorf("buildMessageFTSQuery(%q,%q,%q) leaked a NOT with no positive term: %q",
						c.q, c.hw, c.exclude, got)
				}
			}
		})

		t.Run("positive term present: NOT clauses are expected", func(t *testing.T) {
			// Main query term present.
			if got := buildMessageFTSQuery("hello", "", "spam"); !strings.Contains(got, `NOT "spam"*`) {
				t.Errorf(`buildMessageFTSQuery("hello","","spam") should emit a NOT clause, got %q`, got)
			}
			// hasWords term present (and no main query term) is also a positive
			// term — it must gate the NOT open too, not just the main query.
			if got := buildMessageFTSQuery("", "world", "spam"); !strings.Contains(got, `NOT "spam"*`) {
				t.Errorf(`buildMessageFTSQuery("","world","spam") should emit a NOT clause, got %q`, got)
			}
		})
	})

	t.Run("empty inputs yield empty query", func(t *testing.T) {
		if got := buildMessageFTSQuery("", "", ""); got != "" {
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

// parseSearchRequest must ignore the removed not_words and size_* params even
// if a stale client still sends them — they have no struct fields to land in.
func TestParseSearchRequestIgnoresRemovedParams(t *testing.T) {
	req := httptest.NewRequest(
		"GET",
		"/api/mail/search?from=alice&has_words=invoice&not_words=spam&size_op=gt&size_bytes=1048576&folder=inbox&has_attachment=true",
		nil,
	)
	f := parseSearchRequest(req)

	if f.From != "alice" {
		t.Errorf("From = %q, want alice", f.From)
	}
	if f.HasWords != "invoice" {
		t.Errorf("HasWords = %q, want invoice", f.HasWords)
	}
	if f.Folder != "inbox" {
		t.Errorf("Folder = %q, want inbox", f.Folder)
	}
	if !f.HasAttachment {
		t.Error("HasAttachment = false, want true")
	}
	// The removed params are simply dropped — assert the filter set still
	// reports the surviving structured filters correctly.
	if !hasStructuredFilters(&f) {
		t.Error("hasStructuredFilters() = false, want true (from/folder/attachment set)")
	}
	if !hasAnyFilter(&f) {
		t.Error("hasAnyFilter() = false, want true")
	}
}

// buildMessageWhere must no longer emit a total_size clause.
func TestBuildMessageWhereHasNoSizeClause(t *testing.T) {
	f := api.SearchRequest{From: "alice", Subject: "report", HasAttachment: true}
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
	f := api.SearchRequest{}
	if where := buildMessageWhere(&f, map[string]any{}); where != "" {
		t.Errorf("expected empty WHERE, got %q", where)
	}
}
