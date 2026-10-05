package mail

import (
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
			// render a blank row. The hit carries only ids plus highlights (the
			// client resolves subject/participants/date from its live rows), so
			// the palette's own row — which has no live store behind it — is
			// built from the highlight text with its <mark> markup stripped.
			Title:    titleOr(stripHighlightMarkup(item.SubjectHighlight), "(no subject)"),
			Subtitle: stripHighlightMarkup(item.SnippetHighlight),
			Fields: map[string]any{
				"state_id": item.StateID,
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

// stripHighlightMarkup removes the <mark>/</mark> tags the search SQL wraps
// matched terms in. The palette row has no live store to re-derive plain text
// from, so it renders the highlight text itself, unmarked.
func stripHighlightMarkup(highlighted string) string {
	plain := strings.ReplaceAll(highlighted, "<mark>", "")
	return strings.ReplaceAll(plain, "</mark>", "")
}
