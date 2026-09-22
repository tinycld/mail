package mail

import (
	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"

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
	return nil
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
