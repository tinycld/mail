package mail

import (
	"encoding/json"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/search"
	"tinycld.org/packages/mail/api"
)

// searchSource contributes mail to the federated GET /api/search.
//
// The row mapping here is what the TypeScript adapter's toRow used to own.
// Server-side means the palette and the CLI render identical rows from one
// implementation; a TS version could only ever serve the browser.
//
// Mail keeps its own /api/mail/search route as well: the in-app advanced search
// offers structured filters (from, to, subject, dates, has_attachment, folder)
// that a one-box palette does not, and both call SearchMail — so there is one
// query, not two.
func searchSource() search.Source {
	return search.Source{
		Slug:  "mail",
		Label: "Mail",
		// Mirrors manifest.ts nav.order, the cross-package ranking tie-break.
		Order:  5,
		Scopes: []string{"mail:read"},
		Search: searchMailRows,
	}
}

func searchMailRows(app core.App, userID string, q search.Query) (search.Result, error) {
	resp, err := SearchMail(app, userID, api.SearchRequest{
		Query: strings.Join(q.Include, " "),
		// Mail's own search honors exclusions across both its FTS arms, so a
		// `-term` from the palette reaches SQL rather than being approximated
		// client-side. The positive-term gate lives in the aggregator: an
		// exclude-only query never gets here, because FTS5 errors on a NOT-only
		// MATCH.
		Exclude: strings.Join(q.Exclude, " "),
		Limit:   q.Limit,
		Offset:  q.Offset,
	})
	if err != nil {
		return search.Result{}, err
	}

	rows := make([]search.Row, 0, len(resp.Items))
	for _, item := range resp.Items {
		rows = append(rows, search.Row{
			// Mail's identity is the THREAD, not a message: opening a result
			// opens the conversation. api.SearchResultItem has no `id` field for
			// exactly this reason.
			ID: item.ThreadID,
			// A subject-less thread is still readable, so label it rather than
			// render a blank row.
			Title: titleOr(item.Subject, "(no subject)"),
			// Participants as readable names. The field is the thread's STORED
			// JSON array, so passing it through — as the old TS adapter did —
			// rendered `[{"email":"alice@…` in the palette. The highlighted
			// snippet is deliberately not used instead: it carries <mark>
			// markup, and a CLI would have to strip tags a server sent purely
			// for the web.
			Subtitle: participantNames(item.Participants),
			Meta:     item.LatestDate,
			Fields: map[string]any{
				"mailbox_id":      item.MailboxID,
				"message_count":   item.MessageCount,
				"has_attachments": item.HasAttachments,
			},
		})
	}
	return search.Result{Rows: rows, Total: resp.Total}, nil
}

func titleOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// participantNames turns the thread's stored participants JSON into a readable
// list: "Alice Smith, bob@example.com". Prefers a name, falls back to the
// address, and skips entries with neither.
//
// Unparseable JSON yields an empty subtitle rather than an error: a row with no
// subtitle is still useful, and failing the whole search over one malformed
// column would lose every other result too.
func participantNames(stored string) string {
	if stored == "" {
		return ""
	}
	var people []struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal([]byte(stored), &people); err != nil {
		return ""
	}
	names := make([]string, 0, len(people))
	for _, p := range people {
		if label := firstNonEmpty(p.Name, p.Email); label != "" {
			names = append(names, label)
		}
	}
	const maxShown = 3
	if len(names) > maxShown {
		return strings.Join(names[:maxShown], ", ") + ", …"
	}
	return strings.Join(names, ", ")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
