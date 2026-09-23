package mail

import (
	"fmt"
	"strconv"

	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/logging"
)

var gateLog = logging.ForPackage("mail")

// maxRecipientsPerMessage bounds one outbound message's fan-out.
//
// 100 matches the ceiling the SMTP Rcpt handler already applies to the
// envelope (smtp_session.go), so a compliant mail client sees no change in
// behaviour. It closes the two holes around that check: the HTTP send path
// had no recipient cap of any kind, and on the SMTP path the header To/Cc
// lists — which are what the provider is actually asked to deliver to — were
// never counted at all.
const maxRecipientsPerMessage = 100

// checkSendAllowed is the abuse gate every outbound path must call
// immediately before handing a message to the provider.
//
// It answers one question: may this message go? That is distinct from
// checkSendRateLimit in automation_actions.go, which asks whether a mailbox
// is stuck in an auto-reply loop. Both fail closed; they are not
// interchangeable and neither supersedes the other. A reader who finds one
// should know the other exists — see the note there.
//
// # This fails CLOSED
//
// Every error path here refuses. If a count cannot be computed we do not know
// whether a send is abusive, and an unmetered send is worse than a refused
// one: what is on the other side of the decision is mail leaving under the
// deployment's own domain reputation, which cannot be un-sent.
//
// This is deliberately the opposite of a commercial limit like a seat or
// storage ceiling, which allows a write when its check errors. That trade is
// right for a limit that only decides how much of what someone already owns
// they may use, and wrong here. When you add enforcement to this function,
// the error path refuses.
//
// Takes core.App rather than *pocketbase.PocketBase so it stays testable
// against tests.TestApp.
func checkSendAllowed(app core.App, userID, mailboxID string, recipientCount int) *sendError {
	if recipientCount > maxRecipientsPerMessage {
		gateLog.Warn("refused an outbound message over the recipient cap",
			"user", userID, "mailbox", mailboxID,
			"recipients", recipientCount, "cap", maxRecipientsPerMessage)
		return &sendError{
			kind: sendErrForbidden,
			msg:  "too many recipients: send to at most 100 addresses per message",
		}
	}

	ceiling := dailySendCap(app)
	if ceiling <= 0 {
		return nil
	}

	sent, err := sendsToday(app)
	if err != nil {
		// Fail CLOSED — see the header. We cannot tell whether this send is
		// part of a spam run, and mail that has left cannot be recalled.
		gateLog.Error("could not count today's sends; refusing the send",
			"user", userID, "mailbox", mailboxID, "err", err)
		return &sendError{
			kind: sendErrForbidden,
			msg:  "cannot verify the daily send limit right now; try again shortly",
			err:  err,
		}
	}
	if sent >= ceiling {
		gateLog.Warn("refused an outbound message at the daily send cap",
			"user", userID, "mailbox", mailboxID, "sentToday", sent, "cap", ceiling)
		return &sendError{
			kind: sendErrForbidden,
			msg:  "this deployment has reached its daily send limit; it resets at midnight UTC",
		}
	}

	return nil
}

// dailySendCap resolves the deployment's outbound ceiling for one UTC day.
//
// Read through syscfg rather than the settings collection, which is what puts
// it out of this deployment's own reach: where an operator owns these values
// they are never written here, and a value a deployment could edit is not a
// limit on that deployment. It rides the mail.* namespace the provider
// credentials already use, so an operator sets it exactly where they set the
// Postmark token and no new channel exists to keep in step.
//
// Zero or unset means unlimited, matching every other ceiling in the
// ecosystem. A value that will not parse is treated as unset and logged: the
// alternative is refusing every send over a typo, and the operator gets a
// loud record either way.
func dailySendCap(app core.App) int {
	raw := systemSetting(app, "mail.max_sends_per_day")
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		gateLog.Error("mail.max_sends_per_day is not a non-negative integer; treating it as unlimited",
			"value", raw)
		return 0
	}
	return n
}

// sendsToday counts the messages this deployment has handed to the provider
// since midnight UTC.
//
// Two counting traps here, both learned by checkSendRateLimit before this
// existed, and both silent when you get them wrong.
//
// The bound is a types.DateTime, never an RFC3339 string: PocketBase stores
// dates as "2006-01-02 15:04:05.000Z" and compares them as TEXT, so an
// RFC3339 literal sorts above every stored value and the filter matches
// nothing — a cap that never engages and looks like it works.
//
// Only "sent" and "bounced" count. "sending" names a send in progress but is
// storeMessage's default for an unset status, so every message arriving over
// the inbound webhook or IMAP sync carries it; counting it would charge
// RECEIVED mail against the send budget, and because this check fails closed
// a busy mailbox would read as a silent outage. The synchronous send path
// never leaves a message as "sending", so excluding it loses no coverage.
func sendsToday(app core.App) (int, error) {
	now := types.NowDateTime()
	midnight, err := types.ParseDateTime(now.Time().UTC().Format("2006-01-02") + " 00:00:00.000Z")
	if err != nil {
		return 0, fmt.Errorf("parse midnight: %w", err)
	}

	var result struct {
		Count int `db:"count"`
	}
	err = app.DB().NewQuery(`
		SELECT COUNT(*) AS count FROM mail_messages
		WHERE (delivery_status = 'sent' OR delivery_status = 'bounced')
		  AND date >= {:since}
	`).Bind(map[string]any{"since": midnight}).One(&result)
	if err != nil {
		return 0, fmt.Errorf("count today's sends: %w", err)
	}
	return result.Count, nil
}

// smtpErrorForRefusal maps a gate refusal onto an SMTP reply.
//
// 452 is transient on purpose. A 5xx tells the client the message may never
// be sent, so it hard-bounces mail the user is entitled to send once the list
// is split (or, for a future volume cap, once the window rolls over) — which
// generates a bounce to the sender's own correspondent and damages the very
// reputation this gate exists to protect. 4xx leaves the message in the
// client's retry queue instead.
//
// 4.5.3 ("too many recipients") matches what the Rcpt handler already returns
// for the envelope, so a client that got here with oversized headers sees a
// consistent story.
func smtpErrorForRefusal(e *sendError) *smtp.SMTPError {
	return &smtp.SMTPError{
		Code:         452,
		EnhancedCode: smtp.EnhancedCode{4, 5, 3},
		Message:      e.msg,
	}
}
