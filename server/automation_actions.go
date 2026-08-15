package mail

import (
	"context"
	"fmt"
	"io"
	netmail "net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"tinycld.org/core/automation"
	"tinycld.org/packages/mail/api"
)

// registerAutomationActions installs the Go handlers behind mail's four
// native actions (declared in tinycld/mail/automation.ts). Called from
// registerShared before hooks load, alongside registerAutomationResolver.
//
// Native handlers are NOT pkgaccess-gated — the engine hands them a superuser
// app and only re-applies access checks to record-op actions. Every handler
// here therefore resolves its own audience from the rule's scope and acts
// only for users who are actually members of the message's mailbox.
func registerAutomationActions() {
	automation.RegisterAction("mail:move-to-folder", actionMoveToFolder)
	automation.RegisterAction("mail:mark-as-read", actionMarkAsRead)
	automation.RegisterAction("mail:star-message", actionStarMessage)
	automation.RegisterAction("mail:forward-message", actionForwardMessage)
	automation.RegisterAction("mail:send-message", actionSendMessage)
}

// ruleFolders are the destinations a rule may file a thread into. Narrower
// than the mail_thread_state.folder select (which also permits sent/drafts)
// and must stay in sync with the `folder` param's options in automation.ts —
// a value outside this set is rejected rather than written, so a stale
// declaration can't put a thread somewhere the UI has no view for.
var ruleFolders = map[string]bool{
	"inbox":   true,
	"archive": true,
	"trash":   true,
	"spam":    true,
}

// actionAudience returns the users a rule's action should act for.
//
// Org rules are admin-authored and apply to the whole mailbox: an org
// "archive spam" rule that only tidied its author's view would surprise
// everyone else on a shared mailbox. Personal rules act solely as their
// owner. Either way the result is intersected with actual mailbox
// membership, so a rule can never touch a user who has no claim to the
// message.
func actionAudience(app core.App, req automation.ActionRequest) []string {
	if req.Record == nil {
		return nil
	}

	members := mailboxMembersForMessage(app, req.Record)
	if len(members) == 0 {
		return nil
	}

	scope := ""
	if req.Rule != nil {
		scope = req.Rule.GetString("scope")
	}

	if scope == "org" {
		userIDs := make([]string, 0, len(members))
		for _, member := range members {
			if userID := member.GetString("user"); userID != "" {
				userIDs = append(userIDs, userID)
			}
		}
		return userIDs
	}

	// Personal: the owner, but only if they still belong to the mailbox.
	for _, member := range members {
		if userID := member.GetString("user"); userID != "" && userID == req.OwnerID {
			return []string{req.OwnerID}
		}
	}
	return nil
}

// threadIDForAction returns the thread the triggering message belongs to.
func threadIDForAction(req automation.ActionRequest) (string, error) {
	if req.Record == nil {
		return "", fmt.Errorf("no trigger record")
	}
	threadID := req.Record.GetString("thread")
	if threadID == "" {
		return "", fmt.Errorf("message %s has no thread", req.Record.Id)
	}
	return threadID, nil
}

// actionMoveToFolder files the triggering message's thread into a folder for
// each user in the audience, leaving their read/starred state alone.
func actionMoveToFolder(app core.App, req automation.ActionRequest) error {
	folder := strings.ToLower(strings.TrimSpace(req.Params["folder"]))
	if !ruleFolders[folder] {
		return fmt.Errorf("mail:move-to-folder: unsupported folder %q", folder)
	}

	threadID, err := threadIDForAction(req)
	if err != nil {
		return err
	}

	return applyToAudience(app, req, "mail:move-to-folder", func(userID string) error {
		return setThreadFolder(app, threadID, userID, folder)
	})
}

// applyToAudience runs a per-user thread-state write for every user in the
// rule's audience, and reports a failure only once every user has been tried.
//
// Aborting on the first error would leave an org rule half-applied across a
// shared mailbox — some members' threads filed, the rest untouched, with no
// record of where it stopped. The inbound path already made this call for the
// identical write (endpoints_inbound.go, "Per-member state failure is
// non-fatal"): one user's state row failing says nothing about the next
// user's. The error still surfaces, so the run is recorded as failed and
// auto-disable can still see a rule that fails for everyone.
func applyToAudience(app core.App, req automation.ActionRequest, action string, apply func(userID string) error) error {
	var failed []string
	for _, userID := range actionAudience(app, req) {
		if err := apply(userID); err != nil {
			app.Logger().Error("mail: automation action failed for one user",
				"action", action, "user", userID, "error", err)
			failed = append(failed, userID)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%s: failed for %d user(s): %s", action, len(failed), strings.Join(failed, ", "))
	}
	return nil
}

// actionMarkAsRead clears the unread state for each user in the audience,
// leaving the folder they have the thread filed under alone.
func actionMarkAsRead(app core.App, req automation.ActionRequest) error {
	threadID, err := threadIDForAction(req)
	if err != nil {
		return err
	}

	return applyToAudience(app, req, "mail:mark-as-read", func(userID string) error {
		return setThreadRead(app, threadID, userID, true)
	})
}

// actionStarMessage stars the triggering message's thread for each user in
// the audience, leaving their folder and read state alone.
func actionStarMessage(app core.App, req automation.ActionRequest) error {
	threadID, err := threadIDForAction(req)
	if err != nil {
		return err
	}

	return applyToAudience(app, req, "mail:star-message", func(userID string) error {
		return setThreadStarred(app, threadID, userID, true)
	})
}

// sendAsRuleOwner sends one message through the shared send core on behalf of
// the rule's owner.
//
// The owner is the sender for both scopes: an org rule still has an
// admin-authored owner, and sending as "the whole mailbox" is meaningless —
// a message needs exactly one From. sendMessage re-verifies that the owner
// belongs to the mailbox, which is the access check this native handler
// would otherwise have to make itself.
func sendAsRuleOwner(app core.App, req automation.ActionRequest, to, subject, body string) error {
	if req.OwnerID == "" {
		return fmt.Errorf("no rule owner to send as")
	}

	mailboxID, err := mailboxIDForMessage(app, req.Record)
	if err != nil {
		return err
	}

	recipient, err := ruleRecipient(app, to, mailboxID)
	if err != nil {
		return err
	}

	_, err = sendMessage(app, sendParams{
		Ctx:       context.Background(),
		UserID:    req.OwnerID,
		MailboxID: mailboxID,
		Subject:   subject,
		TextBody:  body,
		To:        []api.Recipient{{Email: recipient}},
	})
	return err
}

// ruleRecipient validates the rule-supplied "to" param and returns the address
// to send to.
//
// The param reaches us after template substitution, so its value is only as
// trustworthy as the triggering message — {{sender_email}} is a documented
// recipe ("reply to whoever mails you"), which means an attacker who can mail
// the mailbox chooses this string. Three things are checked:
//
//   - It parses as exactly one address. mail.ParseAddress rejects the header
//     injection shapes (newlines, a bare comma list) that a raw string would
//     otherwise hand to the provider.
//   - It is not the sending mailbox's own address. That is a direct self-loop
//     the engine's depth cap cannot see: the message re-enters as new inbound
//     mail at depth 0, and only the hourly ceiling would stop it — after 20
//     round trips.
//   - It is non-empty, which ParseAddress already enforces but is worth a
//     distinct error since an unset param is the common authoring mistake.
func ruleRecipient(app core.App, to, mailboxID string) (string, error) {
	raw := strings.TrimSpace(to)
	if raw == "" {
		return "", fmt.Errorf("no recipient")
	}

	parsed, err := netmail.ParseAddress(raw)
	if err != nil {
		return "", fmt.Errorf("invalid recipient %q: %w", raw, err)
	}
	recipient := parsed.Address

	own, err := mailboxAddresses(app, mailboxID)
	if err != nil {
		// A mailbox we cannot resolve is a mailbox we cannot compare against.
		// Blocking is right here for the same reason the rate limiter fails
		// closed — an unverifiable self-send is the loop we are guarding.
		return "", fmt.Errorf("recipient self-send check: %w", err)
	}
	for _, self := range own {
		if strings.EqualFold(recipient, self) {
			return "", fmt.Errorf(
				"recipient %s is delivered to the sending mailbox — refusing to create a mail loop",
				recipient,
			)
		}
	}

	return recipient, nil
}

// mailboxAddresses returns every address that delivers to a mailbox: its
// primary address plus each of its aliases, all as "local@domain".
//
// Aliases have to be in the set. Inbound routing falls back to an alias
// lookup when no mailbox matches the primary address (findMailboxViaAlias,
// via store.go's recipient resolution), so mail addressed to an alias lands
// in this same mailbox — a rule forwarding to one of its own aliases is as
// much a loop as forwarding to the primary address, just harder to spot.
func mailboxAddresses(app core.App, mailboxID string) ([]string, error) {
	mailbox, err := app.FindRecordById("mail_mailboxes", mailboxID)
	if err != nil {
		return nil, fmt.Errorf("mailbox %s not found: %w", mailboxID, err)
	}
	domain, err := app.FindRecordById("mail_domains", mailbox.GetString("domain"))
	if err != nil {
		return nil, fmt.Errorf("domain for mailbox %s not found: %w", mailboxID, err)
	}
	domainName := domain.GetString("domain")

	addresses := []string{mailbox.GetString("address") + "@" + domainName}

	aliases, err := app.FindRecordsByFilter(
		"mail_mailbox_aliases",
		"mailbox = {:mailbox}",
		"",
		0,
		0,
		map[string]any{"mailbox": mailboxID},
	)
	if err != nil {
		return nil, fmt.Errorf("alias lookup for mailbox %s: %w", mailboxID, err)
	}
	for _, alias := range aliases {
		if address := alias.GetString("address"); address != "" {
			addresses = append(addresses, address+"@"+domainName)
		}
	}
	return addresses, nil
}

// mailboxIDForMessage resolves the mailbox a message's thread belongs to.
func mailboxIDForMessage(app core.App, record *core.Record) (string, error) {
	if record == nil {
		return "", fmt.Errorf("no trigger record")
	}
	threadID := record.GetString("thread")
	if threadID == "" {
		return "", fmt.Errorf("message %s has no thread", record.Id)
	}
	thread, err := app.FindRecordById("mail_threads", threadID)
	if err != nil {
		return "", fmt.Errorf("thread %s not found: %w", threadID, err)
	}
	mailboxID := thread.GetString("mailbox")
	if mailboxID == "" {
		return "", fmt.Errorf("thread %s has no mailbox", threadID)
	}
	return mailboxID, nil
}

// actionForwardMessage forwards the triggering message to another address.
func actionForwardMessage(app core.App, req automation.ActionRequest) error {
	if req.Record == nil {
		return fmt.Errorf("mail:forward-message: no trigger record")
	}

	if err := checkSendRateLimit(app, req); err != nil {
		return err
	}

	subject := req.Record.GetString("subject")
	if subject == "" {
		subject = "(no subject)"
	}

	body := forwardedBody(app, req.Record)
	if err := sendAsRuleOwner(app, req, req.Params["to"], "Fwd: "+subject, body); err != nil {
		return fmt.Errorf("mail:forward-message: %w", err)
	}
	return nil
}

// forwardedBody renders the forwarded message as quoted text, with the
// standard header block so the recipient can see who it came from.
//
// Reads the real body rather than the snippet. snippet is truncated to 200
// characters at store time, so forwarding it silently delivered a preview —
// "forward invoices to accounting" sent the first two lines of the invoice.
// The read is one file fetch per forward, which the hourly send ceiling
// already bounds.
func forwardedBody(app core.App, record *core.Record) string {
	var b strings.Builder
	b.WriteString("---------- Forwarded message ----------\n")
	fmt.Fprintf(&b, "From: %s <%s>\n", record.GetString("sender_name"), record.GetString("sender_email"))
	fmt.Fprintf(&b, "Date: %s\n", formatForwardDate(record.GetString("date")))
	fmt.Fprintf(&b, "Subject: %s\n\n", record.GetString("subject"))

	body, err := readMessageBody(app, record)
	if err != nil {
		// Falling back to the snippet keeps the forward useful when the body
		// file is missing or unreadable, which is the case the previous
		// snippet-only implementation always took.
		//
		// One case reaches this path by construction: sendAsRuleOwner passes
		// TextBody only, and storeMessage writes body_html just for HTMLBody,
		// so a rule-sent message has no body file. Forwarding a message that a
		// rule itself sent therefore still truncates at the snippet. Rare
		// enough to accept — it needs two rules chained onto each other — but
		// it is a real edge of this fix, not something the fallback repairs.
		app.Logger().Warn("mail: forward could not read the message body, falling back to the snippet",
			"message", record.Id, "error", err)
		body = record.GetString("snippet")
	}
	b.WriteString(body)
	return b.String()
}

// readMessageBody returns a message's stored body as text.
//
// Bodies are persisted as an HTML file (body_html), so the forwarded text is
// the HTML with tags stripped — the same shape the snippet had, without the
// truncation. Returns an error rather than an empty string so the caller can
// distinguish "no body" from "could not read the body".
func readMessageBody(app core.App, record *core.Record) (string, error) {
	filename := record.GetString("body_html")
	if filename == "" {
		return "", fmt.Errorf("message %s has no body_html file", record.Id)
	}

	fsys, err := app.NewFilesystem()
	if err != nil {
		return "", err
	}
	defer fsys.Close()

	blob, err := fsys.GetReader(record.BaseFilesPath() + "/" + filename)
	if err != nil {
		return "", fmt.Errorf("read body file: %w", err)
	}
	defer blob.Close()

	data, err := io.ReadAll(blob)
	if err != nil {
		return "", err
	}
	return htmlToForwardText(string(data)), nil
}

// blockBoundary matches the tags that end a visual line in an email body.
var blockBoundary = regexp.MustCompile(`(?i)</(?:p|div|tr|h[1-6]|li|blockquote)>|<br\s*/?>`)

// cellBoundary matches the end of a table cell, which separates values on the
// same visual line rather than ending it. Invoices are commonly laid out as
// tables, so without this "forward invoices to accounting" delivers each row's
// cells run together ("Widget12.00" rather than "Widget 12.00").
var cellBoundary = regexp.MustCompile(`(?i)</(?:td|th)>`)

// htmlToForwardText renders body HTML as readable plain text.
//
// stripHTMLToText (sanitize.go) exists for the same job but collapses every
// run of whitespace to a single space, which is right for FTS indexing and
// wrong here — it would deliver a forwarded invoice as one long paragraph.
// Marking block boundaries with newlines before handing off preserves the
// message's line structure through that collapse.
func htmlToForwardText(html string) string {
	// Cells first: </td> becomes a space, so the </tr> that follows still
	// matches and ends the row.
	withBreaks := cellBoundary.ReplaceAllString(html, " ")
	withBreaks = blockBoundary.ReplaceAllString(withBreaks, "\n")

	lines := strings.Split(withBreaks, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, stripHTMLToText(line))
	}

	// Collapse runs of blank lines left by nested block tags, so a
	// </div></p> pair doesn't open a gap in the forwarded text.
	text := strings.Join(out, "\n")
	return strings.TrimSpace(collapseBlankLines.ReplaceAllString(text, "\n\n"))
}

var collapseBlankLines = regexp.MustCompile(`\n{3,}`)

// formatForwardDate renders a stored date for the forwarded header block.
// The DB value is PocketBase's "2006-01-02 15:04:05.000Z"; emitting it raw
// put a machine timestamp in front of the reader.
func formatForwardDate(stored string) string {
	if stored == "" {
		return ""
	}
	parsed, err := time.Parse("2006-01-02 15:04:05.000Z", stored)
	if err != nil {
		return stored
	}
	return parsed.Format(time.RFC1123Z)
}

// actionSendMessage sends a new message with a templated subject and body.
func actionSendMessage(app core.App, req automation.ActionRequest) error {
	if err := checkSendRateLimit(app, req); err != nil {
		return err
	}

	if err := sendAsRuleOwner(app, req, req.Params["to"], req.Params["subject"], req.Params["body"]); err != nil {
		return fmt.Errorf("mail:send-message: %w", err)
	}
	return nil
}

// maxSendsPerMailboxPerHour caps how much rule-generated mail one mailbox can
// emit in an hour.
//
// The engine's depth cap stops a rule re-triggering itself within a single
// dispatch, but it cannot see across dispatches: an auto-reply that answers
// another auto-responder is two systems each doing one hop, forever. An
// hourly ceiling bounds that exchange without stopping legitimate bursts — a
// filter forwarding a morning's invoices stays well under it.
//
// The ceiling is per *mailbox*, not per rule: a loop is a property of the
// address pair, and a per-rule cap multiplies freely with the number of rules
// (N rules on one mailbox each get their own budget, so the mailbox's real
// ceiling was N × the constant). The mailbox is the resource being protected
// and the thing a remote autoresponder is exchanging with.
const maxSendsPerMailboxPerHour = 20

// checkSendRateLimit reports whether the rule has room to send one message.
//
// Counts messages this mailbox has actually sent in the last hour, from
// mail_messages — the same rows the sent folder reads. Three properties
// matter, and each was wrong in an earlier version that counted rule_runs:
//
//   - It counts *sends*, not matched runs. A rule with three send actions
//     writes one rule_runs row but emits three messages, so a run-based count
//     bounded len(actions) × the ceiling rather than the ceiling.
//   - It cannot be pruned out from under itself. pruneRuns keeps only the
//     most recent keepRunsPerRule rows per rule, so a hot rule's older runs
//     were deleted and the cap silently relaxed.
//   - It sees the messages this dispatch already sent. WriteRun is called
//     after the whole action loop, so the in-flight run was never in its own
//     count and the effective cap was off by one.
//
// Called once per message, immediately before each send, so a multi-action
// rule is metered per message rather than per run.
//
// Failing CLOSED on a query error is deliberate, and is the opposite of the
// obvious choice. This is the only control that bounds a cross-dispatch mail
// loop: the depth cap cannot see a hop through an external autoresponder,
// which returns as genuinely new inbound mail at depth 0. If the count is
// unavailable we cannot know whether we are mid-loop, and an unbounded
// auto-reply exchange is a worse failure than a rule that skips a send and
// records the error in its run history.
func checkSendRateLimit(app core.App, req automation.ActionRequest) error {
	if req.Rule == nil {
		return nil
	}

	mailboxID, err := mailboxIDForMessage(app, req.Record)
	if err != nil {
		return fmt.Errorf("send rate-limit check: %w", err)
	}

	// types.DateTime, not an RFC3339 string: PocketBase persists dates as
	// "2006-01-02 15:04:05.000Z" and compares them as text, so an RFC3339
	// literal ("...T01:34:59Z") sorts above every stored value — the filter
	// would silently match nothing and the cap would never engage.
	since := types.NowDateTime().Add(-time.Hour)

	// Only "sent" and "bounced" — the two values deliveryStatusForResult can
	// return (endpoints_send.go). Both mean the message reached the provider,
	// and a bounce still consumed a send.
	//
	// "sending" must NOT be counted, despite naming a send in progress: it is
	// storeMessage's default for an unset status, so every message arriving
	// over the inbound webhook or IMAP sync carries it (see the comment on
	// outboundOrNonReceivedDeliveryStatuses in automation.go). Counting it
	// would charge received mail against the send budget — a mailbox taking 20
	// messages an hour would have no budget left to reply with, and because
	// this check fails closed that reads as a silent outage. The synchronous
	// send path never leaves a message as "sending", so excluding it loses no
	// outbound coverage.
	sent, err := app.FindRecordsByFilter(
		"mail_messages",
		"thread.mailbox = {:mailbox} && (delivery_status = 'sent' || delivery_status = 'bounced') && date >= {:since}",
		"",
		maxSendsPerMailboxPerHour,
		0,
		map[string]any{"mailbox": mailboxID, "since": since},
	)
	if err != nil {
		app.Logger().Error("mail: send rate-limit check failed, blocking send",
			"rule", req.Rule.Id, "mailbox", mailboxID, "error", err)
		return fmt.Errorf(
			"send rate-limit check failed for mailbox %s: %w — blocking the send rather than risk an unbounded auto-reply loop",
			mailboxID, err,
		)
	}

	if len(sent) >= maxSendsPerMailboxPerHour {
		return fmt.Errorf(
			"mailbox %s reached its hourly send limit (%d) — skipping to break a possible auto-reply loop",
			mailboxID, maxSendsPerMailboxPerHour,
		)
	}
	return nil
}
