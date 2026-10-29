package mail

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"

	"tinycld.org/core/davauth"
	"tinycld.org/core/pkgaccess"
	"tinycld.org/core/readonly"
)

// mailboxContext holds the resolved info for a single mailbox membership.
type mailboxContext struct {
	name      string // friendly name for IMAP prefix ("Support")
	mailboxID string
}

// imapSession implements imapserver.SessionIMAP4rev2 backed by PocketBase.
type imapSession struct {
	app core.App

	// Set after Login
	user *core.Record // users record
	// pkgReadonly is the user's org_pkg_access verdict for mail, resolved at
	// Login: reading (SELECT/FETCH/IDLE) stays available, mutating commands
	// refuse. Cached per session — a changed level applies at next login,
	// same as a role change.
	pkgReadonly bool
	// Mailbox index built at login — one entry per mailbox the user can
	// reach, however many member rows give that access
	mailboxIndex []mailboxContext
	multiMailbox bool // true when the user has more than one mailbox

	// Selected state
	selectedMailboxID  string
	selectedFolderName string // bare IMAP folder name (e.g. "INBOX", "Sent", "Labels/Foo")
	// Per-session \Deleted flags (executed on EXPUNGE)
	deleted map[string]bool // message record ID → true
}

var _ imapserver.SessionIMAP4rev2 = (*imapSession)(nil)

func newIMAPSession(app core.App) *imapSession {
	return &imapSession{
		app:     app,
		deleted: make(map[string]bool),
	}
}

// Close cleans up the session.
func (s *imapSession) Close() error {
	s.user = nil
	s.mailboxIndex = nil
	s.multiMailbox = false
	s.selectedMailboxID = ""
	s.selectedFolderName = ""
	s.deleted = nil
	return nil
}

// Login authenticates via PocketBase. davauth.VerifyCredentials is the shared
// protocol-server check: a username or a hosted address (mail's own, through
// addressOwner), and the disabled cutoff (this listener bypasses PocketBase's
// auth hooks, and unlike REST tokens a protocol login never expires). One
// opaque failure keeps the account's state invisible to a prober.
func (s *imapSession) Login(username, password string) error {
	record, err := davauth.VerifyCredentials(s.app, username, password)
	if err != nil {
		return imapserver.ErrAuthFailed
	}

	s.user = record
	s.pkgReadonly = !pkgaccess.CanWrite(s.app, record, "mail")

	mailboxIDs, err := userMailboxIDs(s.app, record.Id)
	if err != nil {
		return fmt.Errorf("failed to load mailbox memberships: %w", err)
	}

	// Build the mailbox index for IMAP folder namespacing. A user can still
	// hold several mailboxes (their personal one plus any shared ones), so
	// batch the lookups: one query for the mailboxes, one for their domains —
	// not two per mailbox. A mailbox whose row or domain row is gone is
	// skipped, as before.
	mailboxes, err := s.app.FindRecordsByIds("mail_mailboxes", mailboxIDs)
	if err != nil {
		return fmt.Errorf("failed to load mailboxes: %w", err)
	}
	mailboxByID := make(map[string]*core.Record, len(mailboxes))
	domainIDs := make([]string, 0, len(mailboxes))
	for _, mailbox := range mailboxes {
		mailboxByID[mailbox.Id] = mailbox
		domainIDs = append(domainIDs, mailbox.GetString("domain"))
	}
	domains, err := s.app.FindRecordsByIds("mail_domains", domainIDs)
	if err != nil {
		return fmt.Errorf("failed to load mail domains: %w", err)
	}
	domainByID := make(map[string]*core.Record, len(domains))
	for _, domain := range domains {
		domainByID[domain.Id] = domain
	}

	for _, mailboxID := range mailboxIDs {
		mailbox := mailboxByID[mailboxID]
		if mailbox == nil {
			continue
		}
		domain := domainByID[mailbox.GetString("domain")]
		if domain == nil {
			continue
		}
		name := mailbox.GetString("name")
		if name == "" {
			name = mailbox.GetString("display_name")
		}
		if name == "" {
			name = mailbox.GetString("address") + "@" + domain.GetString("domain")
		}
		s.mailboxIndex = append(s.mailboxIndex, mailboxContext{
			name: name, mailboxID: mailbox.Id,
		})
	}
	s.multiMailbox = len(s.mailboxIndex) > 1

	return nil
}

// errServerReadOnly refuses a mutating command while the server is
// read-only. The server is then waiting to be replaced, and a write made now
// is lost if the next build rolls back. [UNAVAILABLE] (RFC 5530) tells the
// client that the failure is temporary, so it can keep the change and send
// it again.
var errServerReadOnly = &imap.Error{
	Type: imap.StatusResponseTypeNo,
	Code: imap.ResponseCodeUnavailable,
	Text: "The server is updating; try again in a moment",
}

// requireWritable is the gate of every IMAP command that writes; command
// names it in the log. It refuses a session whose org_pkg_access level is
// not full, and every session while the server is read-only. IMAP bypasses
// the REST layer (where core's request-hook guard and the read-only
// middleware live), so without this a readonly user's client could still
// flag, move, expunge and append — readonly means read — and any client
// could write while the server is read-only.
func (s *imapSession) requireWritable(command string) error {
	if s.pkgReadonly {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "Your mail access is read-only",
		}
	}
	if readonly.Active() {
		imapLog.Info("imap write refused: the server is read-only", "command", command, "user", s.userID())
		return errServerReadOnly
	}
	return nil
}

func (s *imapSession) userID() string {
	if s.user == nil {
		return ""
	}
	return s.user.Id
}

// Namespace returns the personal namespace descriptor.
func (s *imapSession) Namespace() (*imap.NamespaceData, error) {
	return &imap.NamespaceData{
		Personal: []imap.NamespaceDescriptor{{Delim: imapDelim}},
	}, nil
}

// List enumerates available folders.
func (s *imapSession) List(w *imapserver.ListWriter, ref string, patterns []string, options *imap.ListOptions) error {
	if len(patterns) == 0 {
		return w.WriteList(&imap.ListData{
			Attrs: []imap.MailboxAttr{imap.MailboxAttrNoSelect},
			Delim: imapDelim,
		})
	}

	if len(s.mailboxIndex) == 0 {
		return nil
	}

	var allFolders []imap.ListData

	// Read at each LIST, so a changed switch shows at the client's next
	// refresh. A failed read lists the other folders without the views.
	inboxID, views, err := personalAddressViews(s.app, s.user.Id)
	if err != nil {
		imapLog.Warn("could not read the address views", "user", s.user.Id, "error", err)
	}

	for _, ctx := range s.mailboxIndex {
		// Address views belong to the personal inbox only.
		var mailboxViews []addressView
		if ctx.mailboxID == inboxID {
			mailboxViews = views
		}
		folders, err := listUserFolders(s.app, s.user.Id, mailboxViews)
		if err != nil {
			continue
		}

		if s.multiMailbox {
			// Emit the mailbox name as a non-selectable parent
			allFolders = append(allFolders, imap.ListData{
				Mailbox: ctx.name,
				Delim:   imapDelim,
				Attrs:   []imap.MailboxAttr{imap.MailboxAttrNoSelect},
			})
			for _, f := range folders {
				f.Mailbox = ctx.name + "/" + f.Mailbox
				allFolders = append(allFolders, f)
			}
		} else {
			allFolders = append(allFolders, folders...)
		}
	}

	for _, data := range allFolders {
		match := false
		for _, pattern := range patterns {
			if imapserver.MatchList(data.Mailbox, imapDelim, ref, pattern) {
				match = true
				break
			}
		}
		if !match {
			continue
		}

		if options != nil && options.SelectSubscribed {
			data.Attrs = append(data.Attrs, imap.MailboxAttrSubscribed)
		}

		if err := w.WriteList(&data); err != nil {
			return err
		}
	}

	return nil
}

// Select opens a mailbox for access.
func (s *imapSession) Select(name string, options *imap.SelectOptions) (*imap.SelectData, error) {
	ctx, bareName := s.matchMailboxContext(name)
	mailboxID, err := s.resolveFolderWithContext(ctx)
	if err != nil {
		return nil, err
	}

	s.selectedMailboxID = mailboxID
	s.selectedFolderName = bareName
	s.deleted = make(map[string]bool)

	return s.buildSelectData(bareName)
}

// Unselect closes the selected mailbox without expunging.
func (s *imapSession) Unselect() error {
	s.selectedMailboxID = ""
	s.selectedFolderName = ""
	s.deleted = make(map[string]bool)
	return nil
}

// Status returns mailbox status without selecting it.
func (s *imapSession) Status(name string, options *imap.StatusOptions) (*imap.StatusData, error) {
	ctx, bareName := s.matchMailboxContext(name)
	mailboxID, err := s.resolveFolderWithContext(ctx)
	if err != nil {
		return nil, err
	}

	return s.buildStatusData(bareName, mailboxID, options)
}

// Fetch retrieves messages from the selected mailbox.
func (s *imapSession) Fetch(w *imapserver.FetchWriter, numSet imap.NumSet, options *imap.FetchOptions) error {
	messages, err := s.resolveMessages(numSet)
	if err != nil {
		return err
	}

	for seqNum, msg := range messages {
		if err := s.writeMessage(w, uint32(seqNum), msg, options); err != nil {
			return err
		}
	}

	return nil
}

// Search finds messages matching criteria in the selected mailbox.
func (s *imapSession) Search(kind imapserver.NumKind, criteria *imap.SearchCriteria, options *imap.SearchOptions) (*imap.SearchData, error) {
	messages, err := s.selectedMessages()
	if err != nil {
		return nil, err
	}

	// Pre-compute FTS matching for Body/Text criteria to leverage the FTS5
	// index (which has the full body_text, not just the snippet).
	ftsMatchIDs := s.buildFTSMatchSet(criteria)

	var allUIDs []imap.UID
	var allSeqNums []uint32

	for seqNum, msg := range messages {
		if matchesCriteria(s.app, msg, criteria, s.deleted, ftsMatchIDs) {
			uid := imap.UID(msg.GetInt("imap_uid"))
			allUIDs = append(allUIDs, uid)
			allSeqNums = append(allSeqNums, uint32(seqNum))
		}
	}

	data := &imap.SearchData{}
	if kind == imapserver.NumKindUID {
		var uidSet imap.UIDSet
		for _, uid := range allUIDs {
			uidSet.AddNum(uid)
		}
		data.All = uidSet
	} else {
		var seqSet imap.SeqSet
		for _, seq := range allSeqNums {
			seqSet.AddNum(seq)
		}
		data.All = seqSet
	}

	return data, nil
}

// buildFTSMatchSet queries the FTS5 index for Body and Text search criteria,
// returning a set of matching message record IDs. Returns nil if no FTS
// criteria are present (meaning FTS should not constrain results).
func (s *imapSession) buildFTSMatchSet(criteria *imap.SearchCriteria) map[string]bool {
	if criteria == nil {
		return nil
	}

	// Collect all text search terms from Body and Text criteria
	var bodyTerms, textTerms []string
	bodyTerms = append(bodyTerms, criteria.Body...)
	textTerms = append(textTerms, criteria.Text...)

	if len(bodyTerms) == 0 && len(textTerms) == 0 {
		return nil
	}

	db := s.app.DB()

	// RFC 3501: search keys AND together, so each term produces its own match
	// set and the sets intersect. `result` stays nil until the first term
	// constrains it; a term FTS cannot answer (sanitizes away, query error) is
	// skipped and handled below.
	var result map[string]bool
	applyTerm := func(matches map[string]bool) {
		if result == nil {
			result = matches
			return
		}
		for id := range result {
			if !matches[id] {
				delete(result, id)
			}
		}
	}

	// Body searches message body content via FTS
	for _, term := range bodyTerms {
		ftsQuery := sanitizeFTSQuery(term)
		if ftsQuery == "" {
			continue
		}
		var matches []struct {
			RecordID string `db:"record_id"`
		}
		err := db.NewQuery(`
			SELECT record_id FROM fts_mail_messages
			WHERE fts_mail_messages MATCH {:q}
		`).Bind(map[string]any{"q": ftsQuery}).All(&matches)
		if err != nil {
			s.app.Logger().Warn("imap: body FTS query failed", "error", err, "term", term)
			continue
		}
		termSet := make(map[string]bool, len(matches))
		for _, m := range matches {
			termSet[m.RecordID] = true
		}
		applyTerm(termSet)
	}

	// Text searches the whole message — body via fts_mail_messages, plus
	// subject/participants via fts_mail_threads mapped back to messages. The
	// union across the two indexes is one term's matches; terms still AND.
	for _, term := range textTerms {
		ftsQuery := sanitizeFTSQuery(term)
		if ftsQuery == "" {
			continue
		}

		termSet := make(map[string]bool)
		var msgMatches []struct {
			RecordID string `db:"record_id"`
		}
		err := db.NewQuery(`
			SELECT record_id FROM fts_mail_messages
			WHERE fts_mail_messages MATCH {:q}
		`).Bind(map[string]any{"q": ftsQuery}).All(&msgMatches)
		if err == nil {
			for _, m := range msgMatches {
				termSet[m.RecordID] = true
			}
		} else {
			s.app.Logger().Warn("imap: text FTS query failed", "error", err, "term", term)
		}

		// Map matched threads back to their messages in the same statement —
		// a per-thread lookup here was a query per FTS match.
		var threadMsgs []struct {
			ID string `db:"id"`
		}
		err = db.NewQuery(`
			SELECT id FROM mail_messages
			WHERE thread IN (
				SELECT record_id FROM fts_mail_threads
				WHERE fts_mail_threads MATCH {:q}
			)
		`).Bind(map[string]any{"q": ftsQuery}).All(&threadMsgs)
		if err == nil {
			for _, m := range threadMsgs {
				termSet[m.ID] = true
			}
		} else {
			s.app.Logger().Warn("imap: thread FTS query failed", "error", err, "term", term)
		}

		applyTerm(termSet)
	}

	// Criteria were present but no term could constrain (all unanswerable):
	// fail closed — an empty set matches nothing — rather than treating the
	// criteria as absent and matching everything.
	if result == nil {
		return map[string]bool{}
	}
	return result
}

// Store modifies flags on messages.
func (s *imapSession) Store(w *imapserver.FetchWriter, numSet imap.NumSet, flags *imap.StoreFlags, options *imap.StoreOptions) error {
	if err := s.requireWritable("STORE"); err != nil {
		return err
	}

	messages, err := s.resolveMessages(numSet)
	if err != nil {
		return err
	}

	for seqNum, msg := range messages {
		s.applyFlags(msg, flags)

		if !flags.Silent {
			respWriter := w.CreateMessage(uint32(seqNum))
			respWriter.WriteUID(imap.UID(msg.GetInt("imap_uid")))
			respWriter.WriteFlags(s.messageFlags(msg))
			respWriter.Close()
		}
	}

	return nil
}

// Copy copies messages to another mailbox.
func (s *imapSession) Copy(numSet imap.NumSet, dest string) (*imap.CopyData, error) {
	if err := s.requireWritable("COPY"); err != nil {
		return nil, err
	}

	target, err := s.resolveDestination(dest)
	if err != nil {
		return nil, err
	}

	messages, err := s.resolveMessages(numSet)
	if err != nil {
		return nil, err
	}

	var sourceUIDs, destUIDs imap.UIDSet
	for _, msg := range messages {
		if err := s.fileThread(s.app, msg.GetString("thread"), target); err != nil {
			return nil, err
		}
		srcUID := imap.UID(msg.GetInt("imap_uid"))
		sourceUIDs.AddNum(srcUID)
		destUIDs.AddNum(srcUID)
	}

	uidValidity, _ := getMailboxUIDValidity(s.app, target.mailboxID)

	return &imap.CopyData{
		UIDValidity: uidValidity,
		SourceUIDs:  sourceUIDs,
		DestUIDs:    destUIDs,
	}, nil
}

// Move moves messages to another mailbox.
func (s *imapSession) Move(w *imapserver.MoveWriter, numSet imap.NumSet, dest string) error {
	if err := s.requireWritable("MOVE"); err != nil {
		return err
	}

	target, err := s.resolveDestination(dest)
	if err != nil {
		return err
	}

	messages, err := s.resolveMessages(numSet)
	if err != nil {
		return err
	}

	var sourceUIDs, destUIDs imap.UIDSet
	for _, msg := range messages {
		if err := s.fileThread(s.app, msg.GetString("thread"), target); err != nil {
			return err
		}
		srcUID := imap.UID(msg.GetInt("imap_uid"))
		sourceUIDs.AddNum(srcUID)
		destUIDs.AddNum(srcUID)
	}

	destValidity, _ := getMailboxUIDValidity(s.app, s.selectedMailboxID)

	return w.WriteCopyData(&imap.CopyData{
		UIDValidity: destValidity,
		SourceUIDs:  sourceUIDs,
		DestUIDs:    destUIDs,
	})
}

// Append adds a message to a mailbox (e.g., saving a draft or archiving from a client).
func (s *imapSession) Append(mailbox string, r imap.LiteralReader, options *imap.AppendOptions) (*imap.AppendData, error) {
	if err := s.requireWritable("APPEND"); err != nil {
		return nil, err
	}

	target, err := s.resolveDestination(mailbox)
	if err != nil {
		return nil, err
	}
	mailboxID := target.mailboxID

	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read literal: %w", err)
	}

	msg, err := parseRFC5322(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	// Set delivery status based on folder
	switch target.folder {
	case "drafts":
		msg.DeliveryStatus = "draft"
	case "sent":
		msg.DeliveryStatus = "sent"
		msg.SentBy = s.user.Id
	default:
		msg.DeliveryStatus = "delivered"
	}

	// Dedup: if a message with this Message-ID already exists in this mailbox
	// (e.g. SMTP submission already saved it before the client APPENDed), reuse
	// the existing thread and message. Folder/flag updates below still apply,
	// so a subsequent APPEND-to-Drafts of the same message correctly retags.
	existingMsg, existingThread, _ := findMessageInMailbox(s.app, mailboxID, msg.MessageID)
	if existingMsg != nil && target.folder == "sent" && !isOutgoingMessage(existingMsg) {
		// A copy saved to Sent matches only a message this mailbox sent; a
		// received message with the same id (a redirect) stays as it is.
		existingMsg, existingThread = nil, nil
	}

	var thread, record *core.Record
	// One transaction for the whole ingress, mirroring the inbound endpoint.
	// The message create fires the automation engine's record hook, so a
	// move-to-folder rule can write this thread's state row the moment the
	// message is visible. Committing the message before ensureThreadState ran
	// let the rule's write land first and then be overwritten by the default
	// folder here — the user's filter silently undone.
	//
	// A rollback also removes a thread this call created, so there is no
	// partial ingress to clean up.
	err = s.app.RunInTransaction(func(txApp core.App) error {
		if existingMsg != nil {
			thread = existingThread
			record = existingMsg
		} else {
			var err error
			thread, err = findOrCreateThread(txApp, mailboxID, msg.Subject, msg.InReplyTo, "")
			if err != nil {
				return err
			}

			record, err = storeMessage(txApp, thread.Id, msg)
			if err != nil {
				return err
			}

			// Store raw headers
			if len(raw) > 0 {
				headerEnd := bytes.Index(raw, []byte("\r\n\r\n"))
				if headerEnd < 0 {
					headerEnd = bytes.Index(raw, []byte("\n\n"))
				}
				if headerEnd > 0 {
					storeRawHeaders(txApp, record, raw[:headerEnd])
				}
			}

			snippet := msg.TextBody
			if snippet == "" {
				snippet = msg.Subject
			}
			if err := updateThreadMetadata(txApp, thread.Id, msg.SenderName, msg.SenderEmail, snippet, msg.Date); err != nil {
				return err
			}
		}

		if target.folder == "" || target.folder == "sent" {
			// A label or the star, or the client's copy of a message it just
			// sent: the thread stays where it is (a reply must not move the
			// conversation out of the Inbox), and a new thread goes to the Inbox.
			return s.fileThread(txApp, thread.Id, target)
		}
		// A new message is unread unless the client sends \Seen (below).
		return ensureThreadState(txApp, thread.Id, s.user.Id, target.folder, false)
	})
	if err != nil {
		return nil, err
	}

	// Apply flags from APPEND options
	if options != nil && len(options.Flags) > 0 {
		s.applyAppendFlags(thread.Id, options.Flags)
	}

	uid, err := ensureMessageUID(s.app, mailboxID, record)
	if err != nil {
		return nil, err
	}

	uidValidity, _ := getMailboxUIDValidity(s.app, mailboxID)

	// Notify IDLE sessions
	globalNotifier.Notify(mailboxID)

	return &imap.AppendData{
		UID:         imap.UID(uid),
		UIDValidity: uidValidity,
	}, nil
}

// Expunge permanently removes messages marked with \Deleted.
func (s *imapSession) Expunge(w *imapserver.ExpungeWriter, uids *imap.UIDSet) error {
	if err := s.requireWritable("EXPUNGE"); err != nil {
		return err
	}

	messages, err := s.selectedMessages()
	if err != nil {
		return err
	}

	for seqNum, msg := range messages {
		msgID := msg.Id
		uid := imap.UID(msg.GetInt("imap_uid"))

		if !s.deleted[msgID] {
			continue
		}

		if uids != nil && !uids.Contains(uid) {
			continue
		}

		// Move to trash, or permanently delete if already in trash
		threadID := msg.GetString("thread")
		states, err := s.app.FindRecordsByFilter(
			"mail_thread_state",
			"thread = {:thread} && user = {:user}",
			"",
			1,
			0,
			map[string]any{"thread": threadID, "user": s.user.Id},
		)
		if msg.GetString("delivery_status") == "draft" {
			// Clients autosave drafts and expunge the older copies as they go.
			// Trashing the thread would take the whole conversation (a reply
			// draft shares its thread with the mail it answers) to Trash.
			s.app.Delete(msg)
		} else if err == nil && len(states) > 0 {
			if states[0].GetString("folder") == "trash" {
				// Already in trash — permanently delete the message
				s.app.Delete(msg)
			} else {
				// Move to trash
				states[0].Set("folder", "trash")
				s.app.Save(states[0])
			}
		}

		delete(s.deleted, msgID)

		if err := w.WriteExpunge(uint32(seqNum)); err != nil {
			return err
		}
	}

	return nil
}

// Create creates a new mailbox (only label folders supported).
func (s *imapSession) Create(name string, options *imap.CreateOptions) error {
	if err := s.requireWritable("CREATE"); err != nil {
		return err
	}

	_, bareName := s.matchMailboxContext(name)
	if isServerFolder(bareName) {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "Cannot create system folder",
		}
	}
	if !isLabelFolder(bareName) {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "Can only create label folders (Labels/<name>)",
		}
	}
	return createLabelFolder(s.app, bareName)
}

// Delete deletes a mailbox (only label folders supported).
func (s *imapSession) Delete(name string) error {
	if err := s.requireWritable("DELETE"); err != nil {
		return err
	}

	_, bareName := s.matchMailboxContext(name)
	if isServerFolder(bareName) {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "Cannot delete system folder",
		}
	}
	return deleteLabelFolder(s.app, bareName)
}

// Rename renames a mailbox (only label folders supported).
func (s *imapSession) Rename(oldName, newName string, options *imap.RenameOptions) error {
	if err := s.requireWritable("RENAME"); err != nil {
		return err
	}

	_, oldBareName := s.matchMailboxContext(oldName)
	_, newBareName := s.matchMailboxContext(newName)
	if isServerFolder(oldBareName) || isServerFolder(newBareName) {
		return &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "Cannot rename system folder",
		}
	}
	return renameLabelFolder(s.app, oldBareName, newBareName)
}

// Subscribe is a no-op (all folders are always subscribed).
func (s *imapSession) Subscribe(name string) error { return nil }

// Unsubscribe is a no-op.
func (s *imapSession) Unsubscribe(name string) error { return nil }

// Poll checks for updates since the last check.
func (s *imapSession) Poll(w *imapserver.UpdateWriter, allowExpunge bool) error {
	// No-op for now — updates are pushed via Idle
	return nil
}

// Idle waits for changes to the selected mailbox.
func (s *imapSession) Idle(w *imapserver.UpdateWriter, stop <-chan struct{}) error {
	if s.selectedMailboxID == "" {
		<-stop
		return nil
	}

	ch := make(chan struct{}, 1)
	globalNotifier.Subscribe(s.selectedMailboxID, ch)
	defer globalNotifier.Unsubscribe(s.selectedMailboxID, ch)

	for {
		select {
		case <-stop:
			return nil
		case <-ch:
			// Mailbox changed — send EXISTS/RECENT update
			messages, err := s.selectedMessages()
			if err != nil {
				continue
			}
			exists := uint32(len(messages))
			if err := w.WriteNumMessages(exists); err != nil {
				return err
			}
		}
	}
}

// --- Helper methods ---

// errReadOnlyFolder refuses COPY, MOVE and APPEND into a folder whose content
// the server decides (see isReadOnlyFolder).
var errReadOnlyFolder = &imap.Error{
	Type: imap.StatusResponseTypeNo,
	Code: imap.ResponseCodeCannot,
	Text: "Messages cannot be added to this folder",
}

// errNoSuchDestination refuses COPY, MOVE and APPEND into a folder that does
// not exist. RFC 3501 6.4.7: unless the server is certain that the folder
// cannot be created, it must send [TRYCREATE].
var errNoSuchDestination = &imap.Error{
	Type: imap.StatusResponseTypeNo,
	Code: imap.ResponseCodeTryCreate,
	Text: "No such mailbox",
}

// destination is where a COPY, MOVE or APPEND files a thread: exactly one of
// a real folder (a mail_thread_state.folder value), a label, or the star.
type destination struct {
	mailboxID string
	folder    string
	labelID   string
	starred   bool
}

// resolveDestination resolves the folder that a COPY, MOVE or APPEND names,
// before any message changes. A folder whose content the server decides is
// refused with [CANNOT]; an unknown folder or label with [TRYCREATE].
func (s *imapSession) resolveDestination(name string) (destination, error) {
	ctx, bareName := s.matchMailboxContext(name)
	if isReadOnlyFolder(bareName) {
		return destination{}, errReadOnlyFolder
	}
	mailboxID, err := s.resolveFolderWithContext(ctx)
	if err != nil {
		return destination{}, errNoSuchDestination
	}
	target := destination{mailboxID: mailboxID}

	if labelName := extractLabelName(bareName); labelName != "" {
		label, err := findFolderLabel(s.app, labelName, s.user.Id)
		if err != nil {
			return destination{}, err
		}
		if label == nil {
			return destination{}, errNoSuchDestination
		}
		target.labelID = label.Id
		return target, nil
	}

	folder, isVirtual := imapNameToFolder(bareName)
	switch {
	case folder == "starred":
		target.starred = true
	case folder != "" && !isVirtual:
		target.folder = folder
	default:
		return destination{}, errNoSuchDestination
	}
	return target, nil
}

// fileThread files one thread at the destination for this user. A move keeps
// the read state: filing a message says nothing about whether it was read.
func (s *imapSession) fileThread(app core.App, threadID string, target destination) error {
	switch {
	case target.labelID != "":
		return addThreadLabel(app, threadID, s.user.Id, target.labelID)
	case target.starred:
		return setThreadStarred(app, threadID, s.user.Id, true)
	case target.folder == "sent":
		// The Sent folder lists threads flagged sent (see folderToUserFilter).
		return markThreadSent(app, threadID, s.user.Id)
	default:
		return setThreadFolder(app, threadID, s.user.Id, target.folder)
	}
}

// matchMailboxContext matches an IMAP folder name to a mailbox context.
// For multi-mailbox sessions, it strips the mailbox name prefix.
// Returns the matching context and the bare folder name (without prefix).
func (s *imapSession) matchMailboxContext(name string) (mailboxContext, string) {
	if len(s.mailboxIndex) == 0 {
		return mailboxContext{}, name
	}
	if !s.multiMailbox {
		return s.mailboxIndex[0], name
	}
	for _, ctx := range s.mailboxIndex {
		prefix := ctx.name + "/"
		if strings.HasPrefix(name, prefix) {
			return ctx, name[len(prefix):]
		}
		// Exact match on the mailbox name itself (e.g. SELECT "Acme Corp")
		if name == ctx.name {
			return ctx, "INBOX"
		}
	}
	// No prefix match — default to first
	return s.mailboxIndex[0], name
}

// resolveFolderWithContext resolves the mailbox id from a pre-matched context.
func (s *imapSession) resolveFolderWithContext(ctx mailboxContext) (mailboxID string, err error) {
	if ctx.mailboxID == "" {
		return "", &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Code: imap.ResponseCodeNonExistent,
			Text: "No mailbox available",
		}
	}
	return ctx.mailboxID, nil
}

// buildSelectData constructs the response data for a SELECT command.
func (s *imapSession) buildSelectData(name string) (*imap.SelectData, error) {
	messages, err := s.selectedMessages()
	if err != nil {
		return nil, err
	}

	uidValidity, _ := getMailboxUIDValidity(s.app, s.selectedMailboxID)
	uidNext, _ := getMailboxUIDNext(s.app, s.selectedMailboxID)

	numMessages := uint32(len(messages))

	// Count recent (unread) messages
	var numRecent uint32
	for _, msg := range messages {
		threadID := msg.GetString("thread")
		states, err := s.app.FindRecordsByFilter(
			"mail_thread_state",
			"thread = {:thread} && user = {:user} && is_read = false",
			"",
			1,
			0,
			map[string]any{"thread": threadID, "user": s.user.Id},
		)
		if err == nil && len(states) > 0 {
			numRecent++
		}
	}

	flags := []imap.Flag{
		imap.FlagSeen,
		imap.FlagAnswered,
		imap.FlagFlagged,
		imap.FlagDeleted,
		imap.FlagDraft,
	}

	return &imap.SelectData{
		Flags:          flags,
		PermanentFlags: append(flags, imap.Flag(`\*`)),
		NumMessages:    numMessages,
		UIDValidity:    uidValidity,
		UIDNext:        imap.UID(uidNext),
	}, nil
}

// buildStatusData constructs the response data for a STATUS command.
func (s *imapSession) buildStatusData(name, mailboxID string, options *imap.StatusOptions) (*imap.StatusData, error) {
	data := &imap.StatusData{Mailbox: name}

	if options == nil {
		return data, nil
	}

	// go-imap dereferences the pointer of every item the client requested, so
	// each requested item must be set or the whole call must fail. A nil
	// pointer here panics the connection.
	//
	// Every count is of what a SELECT of the folder lists (listedMessages), or
	// a client sees one number in STATUS and another on SELECT.
	if options.NumMessages || options.Size || options.NumUnseen {
		all, err := s.messagesForFolder(name, mailboxID)
		if err != nil {
			return nil, err
		}
		messages := listedMessages(all)
		if options.NumMessages {
			n := uint32(len(messages))
			data.NumMessages = &n
		}
		if options.Size {
			var size int64
			for _, msg := range messages {
				size += int64(msg.GetInt("total_size"))
			}
			data.Size = &size
		}
		if options.NumUnseen {
			// \Seen comes from the thread's read state (messageFlags), so a
			// message is unseen when its thread is unread.
			filter, params := folderToFilter(s.app, name, s.user.Id, mailboxID)
			filter += " && is_read = false"
			states, err := s.app.FindRecordsByFilter("mail_thread_state", filter, "", 0, 0, params)
			if err != nil {
				return nil, err
			}
			unread := make(map[string]bool, len(states))
			for _, state := range states {
				unread[state.GetString("thread")] = true
			}
			var n uint32
			for _, msg := range messages {
				if unread[msg.GetString("thread")] {
					n++
				}
			}
			data.NumUnseen = &n
		}
	}

	if options.UIDValidity {
		v, err := getMailboxUIDValidity(s.app, mailboxID)
		if err != nil {
			return nil, err
		}
		data.UIDValidity = v
	}

	if options.UIDNext {
		n, err := getMailboxUIDNext(s.app, mailboxID)
		if err != nil {
			return nil, err
		}
		data.UIDNext = imap.UID(n)
	}

	// \Deleted marks live only in the selecting session (s.deleted) and are
	// expunged on close, so another folder never holds any. RECENT is
	// obsolete in IMAP4rev2 and this server never sets it. There is no QUOTA
	// support, so no storage is held by deleted messages.
	if options.NumDeleted {
		var zero uint32
		data.NumDeleted = &zero
	}
	if options.NumRecent {
		var zero uint32
		data.NumRecent = &zero
	}
	if options.DeletedStorage {
		var zero int64
		data.DeletedStorage = &zero
	}

	return data, nil
}

// selectedMessages returns all messages visible in the currently selected folder.
func (s *imapSession) selectedMessages() (map[int]*core.Record, error) {
	if s.selectedMailboxID == "" {
		return nil, &imap.Error{
			Type: imap.StatusResponseTypeNo,
			Text: "No mailbox selected",
		}
	}

	// Determine which IMAP folder is selected based on stored state
	// We use the mailbox + user to get thread states, then load messages
	return s.messagesForSelectedFolder()
}

// messagesForSelectedFolder loads messages for whatever folder is currently
// selected. Routes through messagesForFolder so SELECT respects the
// mail_thread_state.folder filter (consistent with STATUS).
func (s *imapSession) messagesForSelectedFolder() (map[int]*core.Record, error) {
	msgs, err := s.messagesForFolder(s.selectedFolderName, s.selectedMailboxID)
	if err != nil {
		return nil, err
	}

	result := make(map[int]*core.Record)
	for i, msg := range listedMessages(msgs) {
		result[i+1] = msg
	}
	return result, nil
}

// listedMessages are the messages a folder lists over IMAP: only those with a
// UID, since a message is addressed by its UID.
func listedMessages(msgs []*core.Record) []*core.Record {
	listed := make([]*core.Record, 0, len(msgs))
	for _, msg := range msgs {
		if msg.GetInt("imap_uid") > 0 {
			listed = append(listed, msg)
		}
	}
	return listed
}

// messagesForFolder loads messages for a specific IMAP folder name.
func (s *imapSession) messagesForFolder(imapName, mailboxID string) ([]*core.Record, error) {
	filter, params := folderToFilter(s.app, imapName, s.user.Id, mailboxID)

	states, err := s.app.FindRecordsByFilter(
		"mail_thread_state",
		filter,
		"",
		0,
		0,
		params,
	)
	if err != nil {
		return nil, err
	}

	// A folder holds whole threads, except Sent, which holds only the
	// outgoing messages of its threads. Listing the received message a reply
	// answered as "sent" put it in Sent and All Mail at once, and clients
	// showed it twice. The filter is isOutgoingMessage as SQL.
	messageFilter := "thread = {:thread}"
	if folder, _ := imapNameToFolder(imapName); folder == "sent" {
		messageFilter += " && (sent_by != '' || delivery_status = 'sent' || delivery_status = 'bounced' || delivery_status = 'spam_complaint')"
	}

	var messages []*core.Record
	for _, state := range states {
		threadID := state.GetString("thread")
		msgs, err := s.app.FindRecordsByFilter(
			"mail_messages",
			messageFilter,
			"imap_uid",
			0,
			0,
			map[string]any{"thread": threadID},
		)
		if err == nil {
			messages = append(messages, msgs...)
		}
	}

	return messages, nil
}

// resolveMessages converts an IMAP NumSet (seq nums or UIDs) to message records.
func (s *imapSession) resolveMessages(numSet imap.NumSet) (map[int]*core.Record, error) {
	allMsgs, err := s.selectedMessages()
	if err != nil {
		return nil, err
	}

	result := make(map[int]*core.Record)

	switch set := numSet.(type) {
	case imap.UIDSet:
		for seqNum, msg := range allMsgs {
			uid := imap.UID(msg.GetInt("imap_uid"))
			if set.Contains(uid) {
				result[seqNum] = msg
			}
		}
	case imap.SeqSet:
		for seqNum, msg := range allMsgs {
			if set.Contains(uint32(seqNum)) {
				result[seqNum] = msg
			}
		}
	}

	return result, nil
}

// writeMessage writes a single message to the FetchWriter.
func (s *imapSession) writeMessage(w *imapserver.FetchWriter, seqNum uint32, msg *core.Record, options *imap.FetchOptions) error {
	respWriter := w.CreateMessage(seqNum)

	uid := imap.UID(msg.GetInt("imap_uid"))
	respWriter.WriteUID(uid)

	if options.Flags {
		respWriter.WriteFlags(s.messageFlags(msg))
	}

	if options.InternalDate {
		respWriter.WriteInternalDate(parseDate(msg.GetString("date")))
	}

	if options.Envelope {
		respWriter.WriteEnvelope(s.buildEnvelope(msg))
	}

	if options.RFC822Size {
		// Estimate size — build the RFC 5322 to get accurate size
		rfc5322, err := buildRFC5322(s.app, msg)
		if err == nil {
			respWriter.WriteRFC822Size(int64(len(rfc5322)))
		}
	}

	// Handle body section fetches
	for _, section := range options.BodySection {
		s.writeBodySection(respWriter, msg, section)
	}

	return respWriter.Close()
}

// writeBodySection handles a BODY[] or BODY[HEADER] fetch.
func (s *imapSession) writeBodySection(respWriter *imapserver.FetchResponseWriter, msg *core.Record, section *imap.FetchItemBodySection) {
	rfc5322, err := buildRFC5322(s.app, msg)
	if err != nil {
		return
	}

	var data []byte
	if len(section.Part) == 0 && section.Specifier == imap.PartSpecifierNone {
		// BODY[] — full message
		data = rfc5322
	} else if section.Specifier == imap.PartSpecifierHeader {
		// BODY[HEADER] — headers only
		headerEnd := bytes.Index(rfc5322, []byte("\r\n\r\n"))
		if headerEnd < 0 {
			headerEnd = bytes.Index(rfc5322, []byte("\n\n"))
		}
		if headerEnd >= 0 {
			data = rfc5322[:headerEnd+4] // include \r\n\r\n
		} else {
			data = rfc5322
		}
	} else if section.Specifier == imap.PartSpecifierText {
		// BODY[TEXT] — body only
		headerEnd := bytes.Index(rfc5322, []byte("\r\n\r\n"))
		if headerEnd >= 0 {
			data = rfc5322[headerEnd+4:]
		}
	} else {
		data = rfc5322
	}

	// Apply partial fetch (octet range)
	if section.Partial != nil {
		offset := int64(section.Partial.Offset)
		if offset >= int64(len(data)) {
			data = nil
		} else {
			count := int64(section.Partial.Size)
			end := offset + count
			if end > int64(len(data)) {
				end = int64(len(data))
			}
			data = data[offset:end]
		}
	}

	wc := respWriter.WriteBodySection(section, int64(len(data)))
	wc.Write(data)
	wc.Close()

	// Mark as seen if not a peek
	if !section.Peek {
		s.markSeen(msg)
	}
}

// buildEnvelope constructs an IMAP Envelope from a message record.
func (s *imapSession) buildEnvelope(msg *core.Record) *imap.Envelope {
	env := &imap.Envelope{
		Date:      parseDate(msg.GetString("date")),
		Subject:   msg.GetString("subject"),
		MessageID: bareMessageID(msg.GetString("message_id")),
		InReplyTo: []string{},
	}

	if irt := bareMessageID(msg.GetString("in_reply_to")); irt != "" {
		env.InReplyTo = []string{irt}
	}

	env.From = []imap.Address{makeIMAPAddress(msg.GetString("sender_name"), msg.GetString("sender_email"))}
	env.Sender = env.From

	env.To = parseIMAPAddresses(msg.GetString("recipients_to"))
	env.Cc = parseIMAPAddresses(msg.GetString("recipients_cc"))

	return env
}

// messageFlags returns the IMAP flags for a message.
func (s *imapSession) messageFlags(msg *core.Record) []imap.Flag {
	var flags []imap.Flag

	threadID := msg.GetString("thread")
	states, _ := s.app.FindRecordsByFilter(
		"mail_thread_state",
		"thread = {:thread} && user = {:user}",
		"",
		1,
		0,
		map[string]any{"thread": threadID, "user": s.user.Id},
	)

	if len(states) > 0 {
		if states[0].GetBool("is_read") {
			flags = append(flags, imap.FlagSeen)
		}
		if states[0].GetBool("is_starred") {
			flags = append(flags, imap.FlagFlagged)
		}
	}

	if msg.GetString("delivery_status") == "draft" {
		flags = append(flags, imap.FlagDraft)
	}

	if s.deleted[msg.Id] {
		flags = append(flags, imap.FlagDeleted)
	}

	return flags
}

// applyFlags modifies thread state based on IMAP flag operations.
func (s *imapSession) applyFlags(msg *core.Record, flags *imap.StoreFlags) {
	threadID := msg.GetString("thread")
	states, err := s.app.FindRecordsByFilter(
		"mail_thread_state",
		"thread = {:thread} && user = {:user}",
		"",
		1,
		0,
		map[string]any{"thread": threadID, "user": s.user.Id},
	)
	if err != nil || len(states) == 0 {
		return
	}
	state := states[0]

	for _, flag := range flags.Flags {
		val := flags.Op == imap.StoreFlagsAdd || flags.Op == imap.StoreFlagsSet
		if flags.Op == imap.StoreFlagsDel {
			val = false
		}

		switch flag {
		case imap.FlagSeen:
			state.Set("is_read", val)
		case imap.FlagFlagged:
			state.Set("is_starred", val)
		case imap.FlagDeleted:
			if val {
				s.deleted[msg.Id] = true
			} else {
				delete(s.deleted, msg.Id)
			}
		}
	}

	s.app.Save(state)
}

// applyAppendFlags applies flags from an APPEND command.
func (s *imapSession) applyAppendFlags(threadID string, flags []imap.Flag) {
	states, err := s.app.FindRecordsByFilter(
		"mail_thread_state",
		"thread = {:thread} && user = {:user}",
		"",
		1,
		0,
		map[string]any{"thread": threadID, "user": s.user.Id},
	)
	if err != nil || len(states) == 0 {
		return
	}
	state := states[0]

	for _, flag := range flags {
		switch flag {
		case imap.FlagSeen:
			state.Set("is_read", true)
		case imap.FlagFlagged:
			state.Set("is_starred", true)
		}
	}
	s.app.Save(state)
}

// markSeen marks the thread as read when a non-peek BODY fetch occurs.
// While the server is read-only the body is still served but the thread
// stays unread: refusing the FETCH would stop all reading for the pause,
// and an unread mark is the harmless side to err on.
func (s *imapSession) markSeen(msg *core.Record) {
	threadID := msg.GetString("thread")
	if readonly.Active() {
		imapLog.Debug("imap fetch did not set \\Seen: the server is read-only", "thread", threadID, "user", s.userID())
		return
	}
	states, err := s.app.FindRecordsByFilter(
		"mail_thread_state",
		"thread = {:thread} && user = {:user}",
		"",
		1,
		0,
		map[string]any{"thread": threadID, "user": s.user.Id},
	)
	if err != nil || len(states) == 0 {
		return
	}
	states[0].Set("is_read", true)
	s.app.Save(states[0])
}

// storeRawHeaders saves original RFC 5322 headers to a message record.
func storeRawHeaders(app core.App, record *core.Record, headers []byte) {
	f, err := filesystem.NewFileFromBytes(headers, "headers.txt")
	if err != nil {
		imapLog.Warn("could not create the raw headers file", "message", record.Id, "error", err)
		return
	}
	record.Set("raw_headers", f)
	if err := app.Save(record); err != nil {
		imapLog.Warn("could not save the raw headers", "message", record.Id, "error", err)
	}
}

// matchesCriteria checks if a message matches IMAP search criteria.
// ftsMatchIDs is a pre-computed set of message record IDs matching Body/Text
// criteria via FTS5. nil means no FTS criteria are present.
func matchesCriteria(app core.App, msg *core.Record, criteria *imap.SearchCriteria, deleted map[string]bool, ftsMatchIDs map[string]bool) bool {
	if criteria == nil {
		return true
	}

	// Check flag criteria
	for _, flag := range criteria.Flag {
		switch flag {
		case imap.FlagSeen:
			// Would need thread state, simplified for now
		case imap.FlagDeleted:
			if !deleted[msg.Id] {
				return false
			}
		}
	}

	for _, flag := range criteria.NotFlag {
		switch flag {
		case imap.FlagDeleted:
			if deleted[msg.Id] {
				return false
			}
		}
	}

	// Check date criteria
	if !criteria.Since.IsZero() {
		msgDate := parseDate(msg.GetString("date"))
		if msgDate.Before(criteria.Since) {
			return false
		}
	}
	if !criteria.Before.IsZero() {
		msgDate := parseDate(msg.GetString("date"))
		if !msgDate.Before(criteria.Before) {
			return false
		}
	}

	// Check header criteria
	for _, hdr := range criteria.Header {
		switch strings.ToLower(hdr.Key) {
		case "subject":
			if !strings.Contains(strings.ToLower(msg.GetString("subject")), strings.ToLower(hdr.Value)) {
				return false
			}
		case "from":
			from := msg.GetString("sender_name") + " " + msg.GetString("sender_email")
			if !strings.Contains(strings.ToLower(from), strings.ToLower(hdr.Value)) {
				return false
			}
		}
	}

	// Check Body/Text criteria via pre-computed FTS match set.
	// If FTS criteria were present (ftsMatchIDs != nil), the message must
	// appear in the set. This uses the FTS5 index which has the full
	// body_text (not just the truncated snippet), giving accurate results.
	if ftsMatchIDs != nil && !ftsMatchIDs[msg.Id] {
		return false
	}

	// Check UID criteria
	if len(criteria.UID) > 0 {
		uid := imap.UID(msg.GetInt("imap_uid"))
		matched := false
		for _, uidSet := range criteria.UID {
			if uidSet.Contains(uid) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check OR criteria
	for _, pair := range criteria.Or {
		if !matchesCriteria(app, msg, &pair[0], deleted, ftsMatchIDs) && !matchesCriteria(app, msg, &pair[1], deleted, ftsMatchIDs) {
			return false
		}
	}

	// Check NOT criteria
	for _, sub := range criteria.Not {
		if matchesCriteria(app, msg, &sub, deleted, ftsMatchIDs) {
			return false
		}
	}

	return true
}

// makeIMAPAddress converts name/email to an imap.Address.
func makeIMAPAddress(name, email string) imap.Address {
	localPart, domain := splitAddress(email)
	return imap.Address{
		Name:    name,
		Mailbox: localPart,
		Host:    domain,
	}
}

// parseIMAPAddresses parses a JSON recipient list into IMAP addresses.
func parseIMAPAddresses(jsonStr string) []imap.Address {
	if jsonStr == "" || jsonStr == "[]" || jsonStr == "null" {
		return nil
	}
	var recipients []Recipient
	if err := json.Unmarshal([]byte(jsonStr), &recipients); err != nil {
		return nil
	}
	addrs := make([]imap.Address, 0, len(recipients))
	for _, r := range recipients {
		addrs = append(addrs, makeIMAPAddress(r.Name, r.Email))
	}
	return addrs
}
