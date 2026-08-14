package mail

import (
	"context"
	"fmt"
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

	for _, userID := range actionAudience(app, req) {
		if err := setThreadFolder(app, threadID, userID, folder); err != nil {
			return fmt.Errorf("mail:move-to-folder: %w", err)
		}
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

	for _, userID := range actionAudience(app, req) {
		if err := setThreadRead(app, threadID, userID, true); err != nil {
			return fmt.Errorf("mail:mark-as-read: %w", err)
		}
	}
	return nil
}

// actionStarMessage stars the triggering message's thread for each user in
// the audience, leaving their folder and read state alone.
func actionStarMessage(app core.App, req automation.ActionRequest) error {
	threadID, err := threadIDForAction(req)
	if err != nil {
		return err
	}

	for _, userID := range actionAudience(app, req) {
		if err := setThreadStarred(app, threadID, userID, true); err != nil {
			return fmt.Errorf("mail:star-message: %w", err)
		}
	}
	return nil
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

	recipient := strings.TrimSpace(to)
	if recipient == "" {
		return fmt.Errorf("no recipient")
	}

	mailboxID, err := mailboxIDForMessage(app, req.Record)
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
func forwardedBody(app core.App, record *core.Record) string {
	var b strings.Builder
	b.WriteString("---------- Forwarded message ----------\n")
	fmt.Fprintf(&b, "From: %s <%s>\n", record.GetString("sender_name"), record.GetString("sender_email"))
	fmt.Fprintf(&b, "Date: %s\n", record.GetString("date"))
	fmt.Fprintf(&b, "Subject: %s\n\n", record.GetString("subject"))

	// The message body lives in a file field, so the snippet is what's
	// readable without a file read on the hot path. Better a short forward
	// than a failed one.
	b.WriteString(record.GetString("snippet"))
	return b.String()
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

// maxSendsPerRulePerHour caps how much mail one rule can emit in an hour.
//
// The engine's depth cap stops a rule re-triggering itself within a single
// dispatch, but it cannot see across dispatches: an auto-reply that answers
// another auto-responder is two systems each doing one hop, forever. A
// per-rule hourly ceiling bounds that exchange without stopping legitimate
// bursts — a filter forwarding a morning's invoices stays well under it.
const maxSendsPerRulePerHour = 20

// checkSendRateLimit reports whether the rule has room to send.
//
// Counts this rule's matched runs in the last hour from rule_runs, the same
// durable log the run-history UI reads, so a restart doesn't reset the
// ceiling the way an in-memory counter would. Failing open on a query error
// is deliberate: a broken count should not silently stop a user's mail.
func checkSendRateLimit(app core.App, req automation.ActionRequest) error {
	if req.Rule == nil {
		return nil
	}

	// types.DateTime, not an RFC3339 string: PocketBase persists dates as
	// "2006-01-02 15:04:05.000Z" and compares them as text, so an RFC3339
	// literal ("...T01:34:59Z") sorts above every stored value — the filter
	// would silently match nothing and the cap would never engage.
	since := types.NowDateTime().Add(-time.Hour)
	runs, err := app.FindRecordsByFilter(
		"rule_runs",
		"rule = {:rule} && matched = true && fired_at >= {:since}",
		"",
		maxSendsPerRulePerHour+1,
		0,
		map[string]any{"rule": req.Rule.Id, "since": since},
	)
	if err != nil {
		app.Logger().Warn("mail: send rate-limit check failed, allowing send",
			"rule", req.Rule.Id, "error", err)
		return nil
	}

	if len(runs) >= maxSendsPerRulePerHour {
		return fmt.Errorf(
			"rule %s reached its hourly send limit (%d) — skipping to break a possible auto-reply loop",
			req.Rule.Id, maxSendsPerRulePerHour,
		)
	}
	return nil
}
