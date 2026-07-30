package mail

import (
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/pocketbase/pocketbase/core"
)

const imapDelim = '/'

// defaultLabelColor is assigned to labels created via IMAP since the protocol
// provides no way for clients to specify a color. Users can recolor from the web UI.
const defaultLabelColor = "#64748b"

// systemFolder describes a built-in IMAP folder mapped to mail_thread_state fields.
type systemFolder struct {
	Name       string
	Attr       imap.MailboxAttr // RFC 6154 SPECIAL-USE attribute
	Selectable bool
}

var systemFolders = []systemFolder{
	{Name: "INBOX", Attr: "", Selectable: true},
	{Name: "Sent", Attr: imap.MailboxAttrSent, Selectable: true},
	{Name: "Drafts", Attr: imap.MailboxAttrDrafts, Selectable: true},
	{Name: "Trash", Attr: imap.MailboxAttrTrash, Selectable: true},
	{Name: "Spam", Attr: imap.MailboxAttrJunk, Selectable: true},
	{Name: "Archive", Attr: imap.MailboxAttrArchive, Selectable: true},
	{Name: "Starred", Attr: imap.MailboxAttrFlagged, Selectable: true},
	{Name: "All Mail", Attr: imap.MailboxAttrAll, Selectable: true},
	{Name: "Labels", Attr: imap.MailboxAttrNoSelect, Selectable: false},
}

// imapNameToFolder maps an IMAP folder name to the PocketBase filter field values.
// Returns: folderValue (for mail_thread_state.folder), isVirtual, filterExpr
func imapNameToFolder(name string) (folder string, isVirtual bool) {
	switch strings.ToLower(name) {
	case "inbox":
		return "inbox", false
	case "sent":
		return "sent", false
	case "drafts":
		return "drafts", false
	case "trash":
		return "trash", false
	case "spam":
		return "spam", false
	case "archive":
		return "archive", false
	case "starred":
		return "starred", true
	case "all mail":
		return "all", true
	default:
		return "", false
	}
}

// folderToFilter builds a PocketBase filter expression for querying
// mail_thread_state records visible in a given IMAP folder of ONE mailbox.
// For label folders, it resolves the label name to a set of thread_state IDs
// via label_assignments (the polymorphic junction the web UI also uses) and
// emits an `id ~ ...` filter. Returns an impossible filter if the label
// doesn't exist or the folder name is unknown.
//
// Every branch pins `thread.mailbox`: thread-state rows are per-user, so a
// user+folder filter alone unions every mailbox the user belongs to — support
// threads showing inside Personal/INBOX, with unseen counts inflated to match.
// The mailbox is the session's SELECTed (or STATUSed) namespace, resolved
// from the folder path prefix.
func folderToFilter(app core.App, imapName, userID, mailboxID string) (string, map[string]any) {
	filter, params := folderToUserFilter(app, imapName, userID)
	if params == nil {
		return filter, params
	}
	params["mailbox"] = mailboxID
	return "thread.mailbox = {:mailbox} && (" + filter + ")", params
}

func folderToUserFilter(app core.App, imapName, userID string) (string, map[string]any) {
	folder, isVirtual := imapNameToFolder(imapName)

	if isVirtual {
		switch folder {
		case "starred":
			return "user = {:user} && is_starred = true",
				map[string]any{"user": userID}
		case "all":
			return "user = {:user}",
				map[string]any{"user": userID}
		}
	}

	// Label folder: Labels/<name> — filter via label_assignments
	if labelName := extractLabelName(imapName); labelName != "" {
		return labelFolderFilter(app, labelName, userID)
	}

	// Standard folder
	if folder != "" {
		return "user = {:user} && folder = {:folder}",
			map[string]any{"user": userID, "folder": folder}
	}

	// Unknown folder — return impossible filter
	return "id = ''", nil
}

// labelFolderFilter resolves a label name to a mail_thread_state id-membership
// filter by joining through label_assignments. Matches both shared labels
// (user = "") and the user's personal labels. Returns an impossible filter
// if the label doesn't exist or has no assignments.
func labelFolderFilter(app core.App, labelName, userID string) (string, map[string]any) {
	labels, err := app.FindRecordsByFilter(
		"labels",
		`name = {:name} && (user = "" || user = {:user})`,
		"",
		1,
		0,
		map[string]any{"name": labelName, "user": userID},
	)
	if err != nil || len(labels) == 0 {
		return "id = ''", nil
	}

	assignments, err := app.FindRecordsByFilter(
		"label_assignments",
		`label = {:label} && collection = "mail_thread_state" && user = {:user}`,
		"",
		0,
		0,
		map[string]any{"label": labels[0].Id, "user": userID},
	)
	if err != nil || len(assignments) == 0 {
		return "id = ''", nil
	}

	ids := make([]string, 0, len(assignments))
	for _, a := range assignments {
		ids = append(ids, a.GetString("record_id"))
	}
	// Emit `id ~ {:i0} || id ~ {:i1} ...` to match the set of thread_state ids.
	params := map[string]any{"user": userID}
	var clauses []string
	for i, id := range ids {
		key := fmt.Sprintf("tid%d", i)
		clauses = append(clauses, fmt.Sprintf("id = {:%s}", key))
		params[key] = id
	}
	return "user = {:user} && (" + strings.Join(clauses, " || ") + ")", params
}

// extractLabelName returns the label name from a "Labels/<name>" folder path,
// or empty string if the name doesn't match the pattern.
func extractLabelName(imapName string) string {
	if strings.HasPrefix(imapName, "Labels/") && len(imapName) > 7 {
		return imapName[7:]
	}
	return ""
}

// isLabelFolder returns true if the IMAP folder name represents a label.
func isLabelFolder(name string) bool {
	return extractLabelName(name) != ""
}

// listUserFolders returns all IMAP folders available to a user, including
// system folders and label-based folders.
// Label folders include shared labels (user = "") and the authenticated
// user's personal labels (user = userID).
func listUserFolders(app core.App, userID string) ([]imap.ListData, error) {
	var result []imap.ListData

	// System folders
	for _, sf := range systemFolders {
		data := imap.ListData{
			Mailbox: sf.Name,
			Delim:   imapDelim,
		}
		if sf.Attr != "" {
			data.Attrs = append(data.Attrs, sf.Attr)
		}
		if !sf.Selectable {
			data.Attrs = append(data.Attrs, imap.MailboxAttrNoSelect)
		}
		result = append(result, data)
	}

	// User's org labels → Labels/<name>
	labels, err := app.FindRecordsByFilter(
		"labels",
		`user = "" || user = {:user}`,
		"name",
		100,
		0,
		map[string]any{"user": userID},
	)
	if err == nil {
		for _, label := range labels {
			result = append(result, imap.ListData{
				Mailbox: "Labels/" + label.GetString("name"),
				Delim:   imapDelim,
			})
		}
	}

	return result, nil
}

// createLabelFolder creates a new shared label for a "Labels/<name>" folder.
// IMAP-created labels are always shared (user = ""); personal labels
// stay a web-UI concept.
func createLabelFolder(app core.App, imapName string) error {
	labelName := extractLabelName(imapName)
	if labelName == "" {
		return fmt.Errorf("invalid label folder name: %s", imapName)
	}

	existing, err := app.FindRecordsByFilter(
		"labels",
		`name = {:name} && user = ""`,
		"",
		1,
		0,
		map[string]any{"name": labelName},
	)
	if err == nil && len(existing) > 0 {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeAlreadyExists,
			Text: "Label already exists",
		}
	}

	collection, err := app.FindCollectionByNameOrId("labels")
	if err != nil {
		return fmt.Errorf("labels collection not found: %w", err)
	}
	record := core.NewRecord(collection)
	record.Set("name", labelName)
	record.Set("color", defaultLabelColor)
	return app.Save(record)
}

// deleteLabelFolder deletes a shared label.
// IMAP can only delete shared labels; personal labels are managed from the web UI.
func deleteLabelFolder(app core.App, imapName string) error {
	labelName := extractLabelName(imapName)
	if labelName == "" {
		return fmt.Errorf("invalid label folder name: %s", imapName)
	}

	labels, err := app.FindRecordsByFilter(
		"labels",
		`name = {:name} && user = ""`,
		"",
		1,
		0,
		map[string]any{"name": labelName},
	)
	if err != nil || len(labels) == 0 {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeNonExistent,
			Text: "Label not found",
		}
	}

	return app.Delete(labels[0])
}

// renameLabelFolder renames a shared label.
// IMAP can only rename shared labels; personal labels are managed from the web UI.
func renameLabelFolder(app core.App, oldName, newName string) error {
	oldLabel := extractLabelName(oldName)
	newLabel := extractLabelName(newName)
	if oldLabel == "" || newLabel == "" {
		return fmt.Errorf("can only rename label folders")
	}

	labels, err := app.FindRecordsByFilter(
		"labels",
		`name = {:name} && user = ""`,
		"",
		1,
		0,
		map[string]any{"name": oldLabel},
	)
	if err != nil || len(labels) == 0 {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeNonExistent,
			Text: "Label not found",
		}
	}

	labels[0].Set("name", newLabel)
	return app.Save(labels[0])
}

// isSystemFolder returns true if the folder name is a built-in system folder
// that cannot be created, deleted, or renamed.
func isSystemFolder(name string) bool {
	for _, sf := range systemFolders {
		if strings.EqualFold(sf.Name, name) {
			return true
		}
	}
	return false
}
