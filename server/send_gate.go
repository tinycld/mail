package mail

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/logging"
	"tinycld.org/core/sendquota"
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
func checkSendAllowed(app core.App, userID, mailboxID string, domain *core.Record, recipientCount int) *sendError {
	if recipientCount > maxRecipientsPerMessage {
		gateLog.Warn("refused an outbound message over the recipient cap",
			"user", userID, "mailbox", mailboxID,
			"recipients", recipientCount, "cap", maxRecipientsPerMessage)
		return &sendError{
			kind: sendErrForbidden,
			msg:  "too many recipients: send to at most 100 addresses per message",
		}
	}

	if refusal := checkDomainAuthenticated(domain); refusal != nil {
		// domain may be nil here — that is one of the states being refused —
		// so the name is resolved defensively rather than off the record.
		domainName := "<unresolved>"
		if domain != nil {
			domainName = domain.GetString("domain")
		}
		gateLog.Warn("refused an outbound message from an unauthenticated domain",
			"user", userID, "mailbox", mailboxID, "domain", domainName)
		return refusal
	}

	limits := sendLimits(app)

	// The hourly ceiling is checked first. It is normally unset, and when it
	// is set this deployment is under review — so that is the reason its
	// sender needs to hear, not a daily number they are nowhere near.
	if limits.PerHour > 0 {
		sent, err := sendsSince(app, time.Now().UTC().Add(-time.Hour))
		if err != nil {
			return uncountableSendRefusal(userID, mailboxID, "hour", err)
		}
		if sent >= limits.PerHour {
			gateLog.Warn("refused an outbound message at the hourly send throttle",
				"user", userID, "mailbox", mailboxID,
				"sentThisHour", sent, "cap", limits.PerHour)
			return &sendError{
				kind: sendErrForbidden,
				msg: "sending from this deployment is temporarily rate-limited while " +
					"its recent delivery problems are reviewed; contact your administrator",
			}
		}
	}

	if limits.PerDay > 0 {
		sent, err := sendsSince(app, startOfUTCDay(time.Now()))
		if err != nil {
			return uncountableSendRefusal(userID, mailboxID, "day", err)
		}
		if sent >= limits.PerDay {
			gateLog.Warn("refused an outbound message at the daily send cap",
				"user", userID, "mailbox", mailboxID, "sentToday", sent, "cap", limits.PerDay)
			return &sendError{
				kind: sendErrForbidden,
				msg:  "this deployment has reached its daily send limit; it resets at midnight UTC",
			}
		}
	}

	return nil
}

// uncountableSendRefusal is the fail-CLOSED path shared by both windows.
//
// Logged at Error rather than Warn: a refusal at a ceiling is the system
// working, while a ceiling that cannot be evaluated is a deployment that has
// stopped sending for a reason nobody asked for.
func uncountableSendRefusal(userID, mailboxID, window string, err error) *sendError {
	gateLog.Error("could not count sends; refusing the send",
		"user", userID, "mailbox", mailboxID, "window", window, "err", err)
	return &sendError{
		kind: sendErrForbidden,
		msg:  "cannot verify the send limit right now; try again shortly",
		err:  err,
	}
}

// checkDomainAuthenticated refuses a send from a domain whose outbound DNS is
// not fully verified.
//
// SPF, DKIM and the return-path CNAME are all records the sender publishes in
// DNS for a domain they control, and the provider confirms each one. That
// makes this the strongest filter in the gate: everything else here bounds how
// much a sender may do, while this one asks whether they own what they claim
// to be sending as. Someone spinning up throwaway accounts to relay spam does
// not have DNS control of a real domain, and cannot get it cheaply.
//
// All three rather than DKIM alone, deliberately. SPF and DKIM are what a
// receiving server actually evaluates, so missing either lands mail in spam
// and teaches the recipient's filter that this deployment sends unauthenticated
// mail — the reputation damage the whole gate exists to prevent. The
// return-path CNAME is what routes bounces back to the provider instead of the
// sender's own MX, which is what makes bounce and complaint handling work at
// all; a deployment that skips it cannot see its own delivery failures.
//
// The flags are refreshed hourly by the re-verification ticker
// (domain_verify_ticker.go), so a domain that has just been configured starts
// sending within the hour without anyone intervening, and one whose DNS is
// later removed stops.
//
// Deliberately NOT gated on `verified`: that field is MX and inbound-domain
// verification — the RECEIVING side. A domain can receive perfectly while
// being unable to prove it owns what it sends as.
func checkDomainAuthenticated(domain *core.Record) *sendError {
	if domain == nil {
		// No domain record means the caller could not resolve one, which is
		// exactly the state this gate must not send from.
		return &sendError{
			kind: sendErrForbidden,
			msg:  "cannot verify the sending domain; check the domain's DNS setup in mail settings",
		}
	}

	var missing []string
	if !domain.GetBool("spf_verified") {
		missing = append(missing, "SPF")
	}
	if !domain.GetBool("dkim_verified") {
		missing = append(missing, "DKIM")
	}
	if !domain.GetBool("return_path_verified") {
		missing = append(missing, "Return-Path")
	}
	if len(missing) == 0 {
		return nil
	}

	return &sendError{
		kind: sendErrForbidden,
		msg: fmt.Sprintf(
			"this domain cannot send yet: %s not verified. Add the DNS records shown in mail settings; verification re-runs hourly.",
			strings.Join(missing, ", ")),
	}
}

// sendLimits resolves this deployment's outbound ceilings.
//
// Read through core's sendquota seam rather than from this deployment's own
// settings, which is the whole point: the hourly ceiling is a throttle
// applied to a deployment whose mail is causing problems, and a throttle its
// subject can raise is not a throttle.
//
// A standalone deployment claims nothing, so the seam resolves to unlimited
// and this falls back to whatever the operator configured for itself. That
// keeps the single-deployment case exactly as it was — a daily cap it sets
// and owns — while a supervised one has both ceilings set for it.
func sendLimits(app core.App) sendquota.SendLimits {
	if sendquota.IsClaimed() {
		return sendquota.Limits(app)
	}
	return sendquota.SendLimits{PerDay: configuredDailyCap(app)}
}

// configuredDailyCap is a standalone deployment's own daily ceiling, read
// from the mail.* settings namespace beside the provider credentials.
//
// Zero or unset means unlimited, matching every other ceiling in the
// ecosystem. A value that will not parse is treated as unset and logged: the
// alternative is refusing every send over a typo, and the operator gets a
// loud record either way.
func configuredDailyCap(app core.App) int {
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

// startOfUTCDay is midnight UTC on the day `now` falls in.
func startOfUTCDay(now time.Time) time.Time {
	u := now.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// sendsSince counts the messages this deployment has handed to the provider
// at or after `since`.
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
func sendsSince(app core.App, since time.Time) (int, error) {
	bound, err := types.ParseDateTime(since.UTC().Format("2006-01-02 15:04:05.000Z"))
	if err != nil {
		return 0, fmt.Errorf("parse window start: %w", err)
	}

	var result struct {
		Count int `db:"count"`
	}
	err = app.DB().NewQuery(`
		SELECT COUNT(*) AS count FROM mail_messages
		WHERE (delivery_status = 'sent' OR delivery_status = 'bounced')
		  AND date >= {:since}
	`).Bind(map[string]any{"since": bound}).One(&result)
	if err != nil {
		return 0, fmt.Errorf("count sends in the window: %w", err)
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
