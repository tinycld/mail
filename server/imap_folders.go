package mail

import (
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

const imapDelim = '/'

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

// folderToFilter builds a PocketBase filter expression for querying threads
// visible in a given IMAP folder for a specific user_org.
func folderToFilter(imapName string, userOrgID string) (string, map[string]any) {
	folder, isVirtual := imapNameToFolder(imapName)

	if isVirtual {
		switch folder {
		case "starred":
			return "user_org = {:userOrg} && is_starred = true",
				map[string]any{"userOrg": userOrgID}
		case "all":
			return "user_org = {:userOrg}",
				map[string]any{"userOrg": userOrgID}
		}
	}

	// Label folder: Labels/<name>
	if labelName := extractLabelName(imapName); labelName != "" {
		return "user_org = {:userOrg} && labels.name ?= {:label}",
			map[string]any{"userOrg": userOrgID, "label": labelName}
	}

	// Standard folder
	if folder != "" {
		return "user_org = {:userOrg} && folder = {:folder}",
			map[string]any{"userOrg": userOrgID, "folder": folder}
	}

	// Unknown folder — return impossible filter
	return "id = ''", nil
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
// system folders and label-based folders from the user's org.
func listUserFolders(app *pocketbase.PocketBase, orgID string) ([]imap.ListData, error) {
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
		"mail_labels",
		"org = {:org}",
		"name",
		100,
		0,
		map[string]any{"org": orgID},
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

// createLabelFolder creates a new label in the org for a "Labels/<name>" folder.
func createLabelFolder(app *pocketbase.PocketBase, orgID, imapName string) error {
	labelName := extractLabelName(imapName)
	if labelName == "" {
		return fmt.Errorf("invalid label folder name: %s", imapName)
	}

	// Check if label already exists
	existing, err := app.FindRecordsByFilter(
		"mail_labels",
		"org = {:org} && name = {:name}",
		"",
		1,
		0,
		map[string]any{"org": orgID, "name": labelName},
	)
	if err == nil && len(existing) > 0 {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeAlreadyExists,
			Text: "Label already exists",
		}
	}

	collection, err := app.FindCollectionByNameOrId("mail_labels")
	if err != nil {
		return fmt.Errorf("mail_labels collection not found: %w", err)
	}
	record := core.NewRecord(collection)
	record.Set("org", orgID)
	record.Set("name", labelName)
	return app.Save(record)
}

// deleteLabelFolder deletes a label from the org.
func deleteLabelFolder(app *pocketbase.PocketBase, orgID, imapName string) error {
	labelName := extractLabelName(imapName)
	if labelName == "" {
		return fmt.Errorf("invalid label folder name: %s", imapName)
	}

	labels, err := app.FindRecordsByFilter(
		"mail_labels",
		"org = {:org} && name = {:name}",
		"",
		1,
		0,
		map[string]any{"org": orgID, "name": labelName},
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

// renameLabelFolder renames a label in the org.
func renameLabelFolder(app *pocketbase.PocketBase, orgID, oldName, newName string) error {
	oldLabel := extractLabelName(oldName)
	newLabel := extractLabelName(newName)
	if oldLabel == "" || newLabel == "" {
		return fmt.Errorf("can only rename label folders")
	}

	labels, err := app.FindRecordsByFilter(
		"mail_labels",
		"org = {:org} && name = {:name}",
		"",
		1,
		0,
		map[string]any{"org": orgID, "name": oldLabel},
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
