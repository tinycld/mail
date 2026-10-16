package mail

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/logging"
)

var imapLog = logging.ForPackage("mail")

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
	// Listed only while the user has address views (see listUserFolders).
	{Name: addressFolderParent, Attr: imap.MailboxAttrNoSelect, Selectable: false},
}

const addressFolderParent = "Addresses"

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

	// Sent is a flag, not a folder: a reply leaves its thread where it was
	// (usually the Inbox) and marks it sent. Trash and spam leave Sent, as in
	// the web view.
	if folder == "sent" {
		return "user = {:user} && is_sent = true && folder != 'trash' && folder != 'spam'",
			map[string]any{"user": userID}
	}

	// Label folder: Labels/<name> — filter via label_assignments
	if labelName := extractLabelName(imapName); labelName != "" {
		return labelFolderFilter(app, labelName, userID)
	}

	if email, ok := extractAddressView(imapName); ok {
		return addressFolderFilter(app, email, userID)
	}

	// Standard folder
	if folder != "" {
		return "user = {:user} && folder = {:folder}",
			map[string]any{"user": userID, "folder": folder}
	}

	// Unknown folder — return impossible filter
	return "id = ''", nil
}

// labelFolderFilter resolves a label name to a mail_thread_state filter that
// keeps the rows the user assigned that label to. Matches both shared labels
// (user = "") and the user's personal labels. Returns an impossible filter
// if the label does not exist.
//
// The join is one @collection alias, so its size does not grow with the
// label: a filter with one clause per assigned thread passed PocketBase's
// 200-expression limit and failed every folder with more threads than that.
// All four conditions share the alias `la`, so they test the same assignment
// row.
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

	return `user = {:user}` +
			` && @collection.label_assignments:la.record_id ?= id` +
			` && @collection.label_assignments:la.label ?= {:label}` +
			` && @collection.label_assignments:la.collection ?= "mail_thread_state"` +
			` && @collection.label_assignments:la.user ?= {:user}`,
		map[string]any{"user": userID, "label": labels[0].Id}
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

// addressView is one address of the personal inbox shown as its own folder.
type addressView struct {
	ID    string
	Email string
}

// addressViewsFor returns the address views of the user's personal inbox.
func addressViewsFor(app core.App, userID string) ([]addressView, error) {
	_, views, err := personalAddressViews(app, userID)
	return views, err
}

// personalAddressViews returns the user's personal inbox and its visible
// addresses, ordered by domain rank, then local part. A view of one address
// is the whole inbox, so with the switch off or fewer than two visible
// addresses there are no views.
func personalAddressViews(app core.App, userID string) (inboxID string, views []addressView, err error) {
	identity, err := app.FindFirstRecordByFilter("mail_identities",
		"user = {:user} && retired != true && inbox != ''", dbx.Params{"user": userID})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("identity lookup: %w", err)
	}
	inboxID = identity.GetString("inbox")
	if !identity.GetBool("address_views") {
		return inboxID, nil, nil
	}
	hidden := identity.GetStringSlice("hidden_address_views")

	addresses, err := app.FindRecordsByFilter("mail_addresses", "mailbox = {:inbox}", "", 0, 0,
		dbx.Params{"inbox": inboxID})
	if err != nil {
		return "", nil, fmt.Errorf("list inbox addresses: %w", err)
	}
	domainIDs := make([]string, 0, len(addresses))
	for _, address := range addresses {
		domainIDs = append(domainIDs, address.GetString("domain"))
	}
	domains, err := app.FindRecordsByIds("mail_domains", domainIDs)
	if err != nil {
		return "", nil, fmt.Errorf("load address domains: %w", err)
	}
	domainByID := make(map[string]*core.Record, len(domains))
	for _, domain := range domains {
		domainByID[domain.Id] = domain
	}

	type ranked struct {
		view  addressView
		rank  int
		local string
	}
	var visible []ranked
	for _, address := range addresses {
		domain := domainByID[address.GetString("domain")]
		if domain == nil || slices.Contains(hidden, address.Id) {
			continue
		}
		local := address.GetString("local_part")
		visible = append(visible, ranked{
			view:  addressView{ID: address.Id, Email: local + "@" + domain.GetString("domain")},
			rank:  domain.GetInt("rank"),
			local: local,
		})
	}
	if len(visible) < 2 {
		return inboxID, nil, nil
	}
	sort.Slice(visible, func(i, j int) bool {
		a, b := visible[i], visible[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.local != b.local {
			return a.local < b.local
		}
		return a.view.Email < b.view.Email
	})
	views = make([]addressView, len(visible))
	for i, r := range visible {
		views[i] = r.view
	}
	return inboxID, views, nil
}

// addressFolderFilter keeps the user's inbox rows whose thread used the
// address. The email resolves only among the user's current views, so a
// client that still has a view selected after the switch went off, or after
// the address was hidden, sees an empty folder instead of an error.
//
// `~` is a substring test on the space-separated id list. Ids are 15
// characters of [a-z0-9], so a match is the whole id.
func addressFolderFilter(app core.App, email, userID string) (string, map[string]any) {
	views, err := addressViewsFor(app, userID)
	if err != nil {
		imapLog.Warn("could not read the address views", "user", userID, "error", err)
		return "id = ''", nil
	}
	for _, view := range views {
		if strings.EqualFold(view.Email, email) {
			return "user = {:user} && folder = 'inbox' && address_keys ~ {:address}",
				map[string]any{"user": userID, "address": view.ID}
		}
	}
	return "id = ''", nil
}

// extractAddressView returns the email of an "Addresses/<email>" folder path.
func extractAddressView(name string) (email string, ok bool) {
	email, ok = strings.CutPrefix(name, addressFolderParent+"/")
	return email, ok && email != ""
}

// isAddressFolder returns true if the IMAP folder name is an address view.
func isAddressFolder(name string) bool {
	_, ok := extractAddressView(name)
	return ok
}

// isReadOnlyFolder reports a folder that messages cannot be copied, moved or
// appended into: a \Noselect parent, or an address view, whose content
// follows the addresses a thread used.
func isReadOnlyFolder(name string) bool {
	if isAddressFolder(name) {
		return true
	}
	for _, sf := range systemFolders {
		if !sf.Selectable && strings.EqualFold(sf.Name, name) {
			return true
		}
	}
	return false
}

// listUserFolders returns all IMAP folders available to a user, including
// system folders and label-based folders.
// Label folders include shared labels (user = "") and the authenticated
// user's personal labels (user = userID). views are the address views to
// list; the caller passes them only for the personal inbox.
func listUserFolders(app core.App, userID string, views []addressView) ([]imap.ListData, error) {
	var result []imap.ListData

	// System folders
	for _, sf := range systemFolders {
		if sf.Name == addressFolderParent && len(views) == 0 {
			continue
		}
		data := imap.ListData{
			Mailbox: sf.Name,
			Delim:   imapDelim,
		}
		if sf.Attr != "" {
			data.Attrs = append(data.Attrs, sf.Attr)
		}
		if !sf.Selectable && sf.Attr != imap.MailboxAttrNoSelect {
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

	for _, view := range views {
		result = append(result, imap.ListData{
			Mailbox: addressFolderParent + "/" + view.Email,
			Delim:   imapDelim,
		})
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

// isServerFolder reports a folder a client cannot create, delete or rename:
// a system folder or an address view.
func isServerFolder(name string) bool {
	return isSystemFolder(name) || isAddressFolder(name)
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
