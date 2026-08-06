package mail

import (
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
	"tinycld.org/packages/mail/api"
)

// searchResultRow is the SQL scan target for the search queries. It stays
// separate from api.SearchResultItem — db tags serve the scan, json tags the
// wire — and TestMapResults_CoversEveryAPIField pins the two field sets
// together.
type searchResultRow struct {
	ThreadID         string `db:"thread_id"`
	Subject          string `db:"subject"`
	SubjectHighlight string `db:"subject_highlight"`
	SnippetHighlight string `db:"snippet_highlight"`
	LatestDate       string `db:"latest_date"`
	Participants     string `db:"participants"`
	MessageCount     int    `db:"message_count"`
	MailboxID        string `db:"mailbox_id"`
	HasAttachments   bool   `db:"has_attachments"`
}

func mapResults(rows []searchResultRow) []api.SearchResultItem {
	items := make([]api.SearchResultItem, len(rows))
	for i, r := range rows {
		items[i] = api.SearchResultItem{
			ThreadID:         r.ThreadID,
			Subject:          r.Subject,
			SubjectHighlight: r.SubjectHighlight,
			SnippetHighlight: r.SnippetHighlight,
			LatestDate:       r.LatestDate,
			Participants:     r.Participants,
			MessageCount:     r.MessageCount,
			MailboxID:        r.MailboxID,
			HasAttachments:   r.HasAttachments,
		}
	}
	return items
}

// threadHasAttachmentsExpr returns a SQL expression that evaluates to 1 if any
// message in thread `t` has attachments, else 0. Aggregated per-thread so the
// indicator is correct regardless of which message matched the FTS query.
const threadHasAttachmentsExpr = `EXISTS (SELECT 1 FROM mail_messages WHERE thread = t.id AND has_attachments = 1)`

func hasStructuredFilters(f *api.SearchRequest) bool {
	return f.From != "" || f.To != "" || f.Subject != "" ||
		f.DateAfter != "" || f.DateBefore != "" ||
		f.HasAttachment || f.Folder != ""
}

func hasAnyFilter(f *api.SearchRequest) bool {
	return hasStructuredFilters(f) || f.HasWords != ""
}

// parseSearchRequest reads the api.SearchRequest contract off the query
// string. Parameters outside the contract are deliberately ignored (removed
// ones like not_words / size_* have no field to land in). Limit is clamped to
// (0, 100] with a default of 25; Offset to >= 0.
func parseSearchRequest(r *http.Request) api.SearchRequest {
	query := r.URL.Query()
	req := api.SearchRequest{
		Query:         query.Get("q"),
		MailboxID:     query.Get("mailbox_id"),
		Limit:         25,
		From:          strings.TrimSpace(query.Get("from")),
		To:            strings.TrimSpace(query.Get("to")),
		Subject:       strings.TrimSpace(query.Get("subject")),
		HasWords:      strings.TrimSpace(query.Get("has_words")),
		DateAfter:     strings.TrimSpace(query.Get("date_after")),
		DateBefore:    strings.TrimSpace(query.Get("date_before")),
		Folder:        strings.TrimSpace(query.Get("folder")),
		HasAttachment: query.Get("has_attachment") == "true",
	}
	if l, err := strconv.Atoi(query.Get("limit")); err == nil && l > 0 && l <= 100 {
		req.Limit = l
	}
	if o, err := strconv.Atoi(query.Get("offset")); err == nil && o >= 0 {
		req.Offset = o
	}
	return req
}

// buildMessageWhere builds additional WHERE clauses and params for mail_messages filters.
func buildMessageWhere(f *api.SearchRequest, params map[string]any) string {
	var clauses []string

	if f.From != "" {
		params["fromLike"] = "%" + f.From + "%"
		clauses = append(clauses, "(m.sender_name LIKE {:fromLike} OR m.sender_email LIKE {:fromLike})")
	}
	if f.To != "" {
		params["toLike"] = "%" + f.To + "%"
		clauses = append(clauses, "(m.recipients_to LIKE {:toLike} OR m.recipients_cc LIKE {:toLike})")
	}
	if f.Subject != "" {
		params["subjectLike"] = "%" + f.Subject + "%"
		clauses = append(clauses, "m.subject LIKE {:subjectLike}")
	}
	if f.DateAfter != "" {
		params["dateAfter"] = f.DateAfter
		clauses = append(clauses, "m.date >= {:dateAfter}")
	}
	if f.DateBefore != "" {
		params["dateBefore"] = f.DateBefore
		clauses = append(clauses, "m.date <= {:dateBefore}")
	}
	if f.HasAttachment {
		clauses = append(clauses, "m.has_attachments = 1")
	}

	if len(clauses) == 0 {
		return ""
	}
	return " AND " + strings.Join(clauses, " AND ")
}

// buildFolderJoin builds an additional JOIN + WHERE clause for folder/starred
// filtering. Single-org: thread state is keyed by the user directly, so this is
// a single bound parameter rather than an IN over the caller's memberships.
func buildFolderJoin(f *api.SearchRequest, userID string, params map[string]any) string {
	if f.Folder == "" || userID == "" {
		return ""
	}

	params["stateUser"] = userID

	if f.Folder == "starred" {
		return " JOIN mail_thread_state ts ON ts.thread = t.id AND ts.user = {:stateUser} AND ts.is_starred = 1"
	}

	params["folder"] = f.Folder
	return " JOIN mail_thread_state ts ON ts.thread = t.id AND ts.user = {:stateUser} AND ts.folder = {:folder}"
}

func handleSearch(app core.App, re *core.RequestEvent) error {
	userID := re.Auth.Id
	filters := parseSearchRequest(re.Request)
	q := filters.Query
	mailboxID := filters.MailboxID
	limit := filters.Limit
	offset := filters.Offset

	emptyResponse := api.SearchResponse{Items: []api.SearchResultItem{}, Total: 0}

	// FTS terms come from the main query OR the Body (hasWords) field. Without
	// counting hasWords here, a body-only search (empty main box, Body filled)
	// would skip the FTS path and fall through to a structured query that
	// ignores hasWords — returning every thread instead of the body matches.
	hasFTSTerms := len(q) >= 2 || filters.HasWords != ""
	hasFilters := hasAnyFilter(&filters)

	if !hasFTSTerms && !hasFilters {
		return re.JSON(http.StatusOK, emptyResponse)
	}

	// A lookup FAILURE must not read as an empty result set — that swallow is
	// what let the ts.user_org rename present as a silent zero. No accessible
	// mailboxes, by contrast, is a genuinely empty answer.
	accessibleMailboxIDs, err := getUserMailboxIDs(app, userID, mailboxID)
	if err != nil {
		app.Logger().Error("search: mailbox lookup failed", "error", err, "user", userID)
		return router.NewApiError(http.StatusInternalServerError, "Search failed", nil)
	}
	if len(accessibleMailboxIDs) == 0 {
		return re.JSON(http.StatusOK, emptyResponse)
	}

	mailboxParams := make(map[string]any)
	mailboxPlaceholders := make([]string, len(accessibleMailboxIDs))
	for i, id := range accessibleMailboxIDs {
		key := "mb" + strconv.Itoa(i)
		mailboxParams[key] = id
		mailboxPlaceholders[i] = "{:" + key + "}"
	}
	inClause := "(" + strings.Join(mailboxPlaceholders, ", ") + ")"

	params := map[string]any{
		"limit":  limit,
		"offset": offset,
	}
	maps.Copy(params, mailboxParams)

	messageWhere := buildMessageWhere(&filters, params)
	folderJoin := buildFolderJoin(&filters, userID, params)

	// SQL-only path: no FTS terms, only structured filters
	if !hasFTSTerms {
		return handleStructuredSearch(app, re, inClause, messageWhere, folderJoin, params, limit, offset)
	}

	// FTS path (possibly with additional structured filters). The two FTS
	// indexes have different columns: fts_mail_threads has no body_text column,
	// so the Body (hasWords) terms only go to the messages query. A body-only
	// search therefore has an empty thread query — we drop that UNION arm rather
	// than run an invalid empty MATCH.
	ftsThreads := buildThreadFTSQuery(q)
	ftsMessages := buildMessageFTSQuery(q, filters.HasWords)
	if ftsThreads == "" && ftsMessages == "" {
		if !hasStructuredFilters(&filters) {
			return re.JSON(http.StatusOK, emptyResponse)
		}
		return handleStructuredSearch(app, re, inClause, messageWhere, folderJoin, params, limit, offset)
	}

	// Only bind a param when its UNION arm is actually present in the SQL — dbx
	// rejects named params that don't appear in the query (which is exactly the
	// body-only case, where the thread arm is dropped).
	if ftsThreads != "" {
		params["ftsThreads"] = ftsThreads
	}
	if ftsMessages != "" {
		params["ftsMessages"] = ftsMessages
	}

	// When message-level filters are active, use EXISTS to avoid row multiplication
	msgExistsClause := ""
	if messageWhere != "" {
		msgExistsClause = " AND EXISTS (SELECT 1 FROM mail_messages m WHERE m.thread = t.id" + messageWhere + ")"
	}

	threadQuery := `
		SELECT
			t.id as thread_id,
			t.subject,
			highlight(fts_mail_threads, 1, '<mark>', '</mark>') as subject_highlight,
			snippet(fts_mail_threads, 2, '<mark>', '</mark>', '...', 40) as snippet_highlight,
			t.latest_date,
			t.participants,
			t.message_count,
			t.mailbox as mailbox_id,
			` + threadHasAttachmentsExpr + ` as has_attachments,
			fts_mail_threads.rank
		FROM fts_mail_threads
		JOIN mail_threads t ON t.id = fts_mail_threads.record_id` + folderJoin + `
		WHERE fts_mail_threads MATCH {:ftsThreads}
		AND t.mailbox IN ` + inClause + msgExistsClause

	messageQuery := `
		SELECT
			t.id as thread_id,
			t.subject,
			'' as subject_highlight,
			snippet(fts_mail_messages, 5, '<mark>', '</mark>', '...', 40) as snippet_highlight,
			t.latest_date,
			t.participants,
			t.message_count,
			t.mailbox as mailbox_id,
			` + threadHasAttachmentsExpr + ` as has_attachments,
			fts_mail_messages.rank
		FROM fts_mail_messages
		JOIN mail_messages m ON m.id = fts_mail_messages.record_id
		JOIN mail_threads t ON t.id = m.thread` + folderJoin + `
		WHERE fts_mail_messages MATCH {:ftsMessages}
		AND t.mailbox IN ` + inClause + messageWhere

	unionBody := ftsUnion(ftsThreads != "", threadQuery, ftsMessages != "", messageQuery)

	combinedQuery := `
		SELECT thread_id, MAX(subject) as subject,
			   MAX(subject_highlight) as subject_highlight,
			   MAX(snippet_highlight) as snippet_highlight,
			   MAX(latest_date) as latest_date,
			   MAX(participants) as participants,
			   MAX(message_count) as message_count,
			   MAX(mailbox_id) as mailbox_id,
			   MAX(has_attachments) as has_attachments
		FROM (
			` + unionBody + `
		)
		GROUP BY thread_id
		ORDER BY MIN(rank)
		LIMIT {:limit} OFFSET {:offset}
	`

	var results []searchResultRow
	err = app.DB().NewQuery(combinedQuery).Bind(dbx.Params(params)).All(&results)
	if err != nil {
		app.Logger().Error("FTS: search query failed", "error", err, "query", q)
		return router.NewApiError(http.StatusInternalServerError, "Search failed", nil)
	}

	items := mapResults(results)
	total := len(items)
	if len(items) >= limit {
		countThreadArm := `
				SELECT t.id as thread_id
				FROM fts_mail_threads
				JOIN mail_threads t ON t.id = fts_mail_threads.record_id` + folderJoin + `
				WHERE fts_mail_threads MATCH {:ftsThreads}
				AND t.mailbox IN ` + inClause + msgExistsClause
		countMessageArm := `
				SELECT t.id as thread_id
				FROM fts_mail_messages
				JOIN mail_messages m ON m.id = fts_mail_messages.record_id
				JOIN mail_threads t ON t.id = m.thread` + folderJoin + `
				WHERE fts_mail_messages MATCH {:ftsMessages}
				AND t.mailbox IN ` + inClause + messageWhere
		var countArms []string
		if ftsThreads != "" {
			countArms = append(countArms, countThreadArm)
		}
		if ftsMessages != "" {
			countArms = append(countArms, countMessageArm)
		}
		countQuery := `
			SELECT COUNT(DISTINCT thread_id) as total FROM (
				` + strings.Join(countArms, "\n\t\t\t\tUNION\n\t\t\t\t") + `
			)
		`
		var countResult struct {
			Total int `db:"total"`
		}
		countParams := map[string]any{}
		// Bind only the arms present in the count SQL (same dbx constraint as
		// the main query).
		if ftsThreads != "" {
			countParams["ftsThreads"] = ftsThreads
		}
		if ftsMessages != "" {
			countParams["ftsMessages"] = ftsMessages
		}
		maps.Copy(countParams, mailboxParams)
		// Copy filter params needed for WHERE clauses
		for k, v := range params {
			if k != "limit" && k != "offset" && k != "ftsThreads" && k != "ftsMessages" {
				countParams[k] = v
			}
		}
		if err := app.DB().NewQuery(countQuery).Bind(dbx.Params(countParams)).One(&countResult); err == nil {
			total = countResult.Total
		}
	} else if offset > 0 {
		total = offset + len(items)
	}

	return re.JSON(http.StatusOK, api.SearchResponse{Items: items, Total: total})
}

// handleStructuredSearch runs a SQL-only search (no FTS) when only structured filters are present.
func handleStructuredSearch(
	app core.App,
	re *core.RequestEvent,
	inClause, messageWhere, folderJoin string,
	params map[string]any,
	limit, offset int,
) error {
	query := `
		SELECT DISTINCT
			t.id as thread_id,
			t.subject,
			'' as subject_highlight,
			t.snippet as snippet_highlight,
			t.latest_date,
			t.participants,
			t.message_count,
			t.mailbox as mailbox_id,
			` + threadHasAttachmentsExpr + ` as has_attachments
		FROM mail_threads t
		JOIN mail_messages m ON m.thread = t.id` + folderJoin + `
		WHERE t.mailbox IN ` + inClause + messageWhere + `
		ORDER BY t.latest_date DESC
		LIMIT {:limit} OFFSET {:offset}
	`

	var results []searchResultRow
	err := app.DB().NewQuery(query).Bind(dbx.Params(params)).All(&results)
	if err != nil {
		app.Logger().Error("Structured search failed", "error", err)
		return router.NewApiError(http.StatusInternalServerError, "Search failed", nil)
	}

	items := mapResults(results)
	total := len(items)
	if len(items) >= limit {
		countQuery := `
			SELECT COUNT(DISTINCT t.id) as total
			FROM mail_threads t
			JOIN mail_messages m ON m.thread = t.id` + folderJoin + `
			WHERE t.mailbox IN ` + inClause + messageWhere + `
		`
		countParams := make(map[string]any)
		for k, v := range params {
			if k != "limit" && k != "offset" {
				countParams[k] = v
			}
		}
		var countResult struct {
			Total int `db:"total"`
		}
		if err := app.DB().NewQuery(countQuery).Bind(dbx.Params(countParams)).One(&countResult); err == nil {
			total = countResult.Total
		}
	} else if offset > 0 {
		total = offset + len(items)
	}

	return re.JSON(http.StatusOK, api.SearchResponse{Items: items, Total: total})
}

// getUserMailboxIDs returns the mailbox IDs the user has access to. If mailboxID
// is provided, it filters to just that mailbox (after verifying access).
// Single-org: membership rows point at the user directly, so this is one query.
func getUserMailboxIDs(app core.App, userID, mailboxID string) ([]string, error) {
	members, err := app.FindRecordsByFilter(
		"mail_mailbox_members",
		"user = {:user}",
		"",
		100,
		0,
		map[string]any{"user": userID},
	)
	if err != nil {
		return nil, err
	}

	allMailboxIDs := make([]string, 0, len(members))
	for _, m := range members {
		allMailboxIDs = append(allMailboxIDs, m.GetString("mailbox"))
	}

	if mailboxID != "" {
		if slices.Contains(allMailboxIDs, mailboxID) {
			return []string{mailboxID}, nil
		}
		return nil, nil
	}

	return allMailboxIDs, nil
}
