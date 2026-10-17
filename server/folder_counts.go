package mail

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/logging"
)

var folderCountsLog = logging.ForPackage("mail")

// folderCountColumns are the per-folder numbers on a mail_folder_counts row.
// `total` is All Mail; `all` is an SQL keyword.
var folderCountColumns = []string{
	"inbox", "inbox_total", "archive", "drafts", "sent", "starred", "trash", "spam", "total",
}

// Sent is a flag, not a folder, and a thread in trash or spam leaves Sent.
// Mirrors the Sent predicate the client's folderTerms compiles. SUM over no
// rows is NULL, which will not scan into an int, so COALESCE keeps the
// recompute after a mailbox's last state row is deleted from failing.
const folderCountsQuery = `
	SELECT
		COALESCE(SUM(CASE WHEN folder = 'inbox' AND is_read = 0 THEN 1 ELSE 0 END), 0) AS inbox,
		COALESCE(SUM(CASE WHEN folder = 'inbox' THEN 1 ELSE 0 END), 0) AS inbox_total,
		COALESCE(SUM(CASE WHEN folder = 'archive' THEN 1 ELSE 0 END), 0) AS archive,
		COALESCE(SUM(CASE WHEN folder = 'drafts' THEN 1 ELSE 0 END), 0) AS drafts,
		COALESCE(SUM(CASE WHEN is_sent AND folder NOT IN ('trash', 'spam') THEN 1 ELSE 0 END), 0) AS sent,
		COALESCE(SUM(CASE WHEN is_starred THEN 1 ELSE 0 END), 0) AS starred,
		COALESCE(SUM(CASE WHEN folder = 'trash' THEN 1 ELSE 0 END), 0) AS trash,
		COALESCE(SUM(CASE WHEN folder = 'spam' THEN 1 ELSE 0 END), 0) AS spam,
		COUNT(*) AS total
	FROM mail_thread_state
	WHERE user = {:user} AND mailbox = {:mailbox}`

// Only the user's personal inbox (the one their identity points at) gets
// per-address numbers, so the identity test sits in this query: one round trip
// per recompute, and a shared inbox matches no rows. instr over the space-padded
// list matches whole ids only.
const addressCountsQuery = `
	SELECT
		a.id AS address,
		COALESCE(SUM(CASE WHEN s.is_read = 0 THEN 1 ELSE 0 END), 0) AS unread,
		COUNT(s.id) AS total
	FROM mail_addresses a
	JOIN mail_thread_state s ON s.user = {:user} AND s.mailbox = {:mailbox} AND s.folder = 'inbox'
		AND instr(' ' || s.address_keys || ' ', ' ' || a.id || ' ') > 0
	WHERE a.mailbox = {:mailbox}
		AND EXISTS (SELECT 1 FROM mail_identities i WHERE i.user = {:user} AND i.inbox = {:mailbox})
	GROUP BY a.id`

type addressCountsRow struct {
	Address string `db:"address"`
	Unread  int    `db:"unread"`
	Total   int    `db:"total"`
}

func inboxByAddress(app core.App, userID, mailboxID string) (string, error) {
	var rows []addressCountsRow
	if err := app.DB().NewQuery(addressCountsQuery).
		Bind(map[string]any{"user": userID, "mailbox": mailboxID}).
		All(&rows); err != nil {
		return "", fmt.Errorf("aggregate address counts: %w", err)
	}
	byAddress := make(map[string]addressCount, len(rows))
	for _, r := range rows {
		byAddress[r.Address] = addressCount{Unread: r.Unread, Total: r.Total}
	}
	// Marshal sorts map keys, so equal maps give equal text and the compare
	// against the stored value is a string compare.
	encoded, err := json.Marshal(byAddress)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

type folderCountsRow struct {
	Inbox      int `db:"inbox"`
	InboxTotal int `db:"inbox_total"`
	Archive    int `db:"archive"`
	Drafts     int `db:"drafts"`
	Sent       int `db:"sent"`
	Starred    int `db:"starred"`
	Trash      int `db:"trash"`
	Spam       int `db:"spam"`
	Total      int `db:"total"`
}

// addressCount is one address's numbers inside inbox_by_address.
type addressCount struct {
	Unread int `json:"unread"`
	Total  int `json:"total"`
}

func (r folderCountsRow) byColumn() map[string]int {
	return map[string]int{
		"inbox": r.Inbox, "inbox_total": r.InboxTotal, "archive": r.Archive, "drafts": r.Drafts,
		"sent": r.Sent, "starred": r.Starred, "trash": r.Trash, "spam": r.Spam, "total": r.Total,
	}
}

// folderCountsID is the row id for one (user, mailbox): the two 15-char ids
// joined, so a recompute addresses its row without a lookup.
func folderCountsID(userID, mailboxID string) string {
	return userID + mailboxID
}

// registerFolderCountHooks recomputes the (user, mailbox) counts row after
// every mail_thread_state change. One aggregate query per change; a bulk
// archive of N rows recomputes N times, which is accepted. Record saves, so
// PocketBase emits realtime events and the sidebar needs no bridge.
func registerFolderCountHooks(app core.App) {
	recompute := func(e *core.RecordEvent) error {
		if err := recomputeFolderCounts(e.App, e.Record.GetString("user"), e.Record.GetString("mailbox")); err != nil {
			folderCountsLog.Warn("recomputeFolderCounts failed", "state", e.Record.Id, "error", err)
		}
		return e.Next()
	}
	app.OnRecordAfterCreateSuccess("mail_thread_state").BindFunc(recompute)
	// Only the new (user, mailbox) is recomputed: a state's mailbox is copied
	// from its thread and never changes.
	app.OnRecordAfterUpdateSuccess("mail_thread_state").BindFunc(recompute)
	app.OnRecordAfterDeleteSuccess("mail_thread_state").BindFunc(recompute)

	// The sidebar shows the per-address numbers once the views are on, so
	// they must be current then, even when no state row has changed since
	// the migration added them.
	app.OnRecordAfterUpdateSuccess("mail_identities").BindFunc(func(e *core.RecordEvent) error {
		if !addressViewsChanged(e.Record) {
			return e.Next()
		}
		if err := recomputeFolderCounts(e.App, e.Record.GetString("user"), e.Record.GetString("inbox")); err != nil {
			folderCountsLog.Warn("recomputeFolderCounts after an address views change failed",
				"identity", e.Record.Id, "error", err)
		}
		return e.Next()
	})
}

// errFolderCountsRowTaken means another change of the same mailbox created
// the counts row between this recompute's lookup and its create.
var errFolderCountsRowTaken = errors.New("the folder counts row was created by another change")

// recomputeFolderCounts aggregates the user's state rows in one mailbox and
// writes the counts row, creating it on first use. Saves only when a number
// changed.
//
// The lookup and the create are not one step, so two changes of a mailbox
// that has no row yet (a burst of new threads) can both try to create it.
// The one that loses runs once more: the row then exists, and the second run
// reads the aggregate again and updates the row.
func recomputeFolderCounts(app core.App, userID, mailboxID string) error {
	err := writeFolderCounts(app, userID, mailboxID)
	if errors.Is(err, errFolderCountsRowTaken) {
		return writeFolderCounts(app, userID, mailboxID)
	}
	return err
}

func writeFolderCounts(app core.App, userID, mailboxID string) error {
	if userID == "" || mailboxID == "" {
		return nil
	}
	var row folderCountsRow
	if err := app.DB().NewQuery(folderCountsQuery).
		Bind(map[string]any{"user": userID, "mailbox": mailboxID}).
		One(&row); err != nil {
		return fmt.Errorf("aggregate folder counts: %w", err)
	}

	byAddress, err := inboxByAddress(app, userID, mailboxID)
	if err != nil {
		return err
	}

	id := folderCountsID(userID, mailboxID)
	record, err := app.FindRecordById("mail_folder_counts", id)
	if err != nil {
		// No state rows and no counts row: nothing to show. This is also the
		// path after a mailbox or user delete, where the cascade removed the
		// counts row and creating one would point at the deleted record.
		if row.Total == 0 {
			return nil
		}
		collection, err := app.FindCollectionByNameOrId("mail_folder_counts")
		if err != nil {
			return fmt.Errorf("mail_folder_counts collection: %w", err)
		}
		record = core.NewRecord(collection)
		record.Id = id
		record.Set("user", userID)
		record.Set("mailbox", mailboxID)
	}

	changed := record.IsNew()
	for name, value := range row.byColumn() {
		if record.GetInt(name) != value {
			record.Set(name, value)
			changed = true
		}
	}
	if record.GetString("inbox_by_address") != byAddress {
		record.Set("inbox_by_address", json.RawMessage(byAddress))
		changed = true
	}
	if !changed {
		return nil
	}
	isNew := record.IsNew()
	err = app.Save(record)
	if err != nil && isNew {
		if _, findErr := app.FindRecordById("mail_folder_counts", id); findErr == nil {
			return errFolderCountsRowTaken
		}
	}
	return err
}

// addressViewsChanged reports an identity save that turned the address views
// on or changed which ones are hidden.
func addressViewsChanged(identity *core.Record) bool {
	original := identity.Original()
	if identity.GetBool("address_views") && !original.GetBool("address_views") {
		return true
	}
	return !slices.Equal(identity.GetStringSlice("hidden_address_views"), original.GetStringSlice("hidden_address_views"))
}

// unfilledAddressCountsQuery finds the personal inboxes with the views on
// whose counts row has no per-address numbers yet. The address views
// migration added the column without values, and the numbers are otherwise
// written only when a state row changes.
const unfilledAddressCountsQuery = `
	SELECT i.user AS user, i.inbox AS inbox
	FROM mail_identities i
	JOIN mail_folder_counts c ON c.user = i.user AND c.mailbox = i.inbox
	WHERE i.address_views = 1 AND i.inbox != ''
		AND (c.inbox_by_address IS NULL OR c.inbox_by_address IN ('', 'null'))`

// backfillAddressViewCounts fills the per-address numbers of every counts
// row the migration left without them. A filled row holds at least "{}", so
// a later run finds nothing to do.
func backfillAddressViewCounts(app core.App) error {
	var rows []struct {
		User  string `db:"user"`
		Inbox string `db:"inbox"`
	}
	if err := app.DB().NewQuery(unfilledAddressCountsQuery).All(&rows); err != nil {
		return fmt.Errorf("find counts rows without address numbers: %w", err)
	}
	var errs []error
	for _, row := range rows {
		if err := recomputeFolderCounts(app, row.User, row.Inbox); err != nil {
			errs = append(errs, fmt.Errorf("user %s: %w", row.User, err))
		}
	}
	return errors.Join(errs...)
}
