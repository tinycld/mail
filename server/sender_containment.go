package mail

import (
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/notify"
)

// The most likely reason a deployment's mail suddenly goes wrong is ONE
// account, not the organisation. A phished password, a leaked API token, a
// laptop somebody else is using — and the organisation around it is doing
// nothing different from last week.
//
// Limiting everyone's sending in that case punishes the wrong people and does
// not even stop the cause: the compromised account still holds whatever
// budget the limit leaves. Disabling that one account does stop it, and
// leaves everyone else working.
//
// This is only possible because mail_messages.sent_by is server-owned and
// unforgeable — see sent_by_guard.go, which exists precisely so a member
// cannot make their sending look like somebody else's. Attribution that the
// attributed party could edit would make this a weapon rather than a
// containment.

// senderConcentration is the share of recent failures that must trace to one
// account before that account is held responsible for them.
//
// High on purpose. Below this the failures are spread across the
// organisation's ordinary sending, which is a list problem or a reputation
// problem and not one account's doing — and disabling somebody over a
// coincidence is worse than not acting, because they cannot undo it
// themselves.
const senderConcentration = 0.8

// minFailuresToAttribute is the floor below which concentration means
// nothing. Two failures from one person is a Tuesday; it is only a pattern
// once there are enough of them that one account holding most of them cannot
// be chance.
const minFailuresToAttribute = 5

// ConcentratedSender reports the user that most recent delivery failures
// trace to, when one account dominates them.
//
// Returns "" when nothing dominates, which is the common case and the safe
// answer: no finding beats a wrong one when the consequence is locking
// somebody out of their own account.
func ConcentratedSender(app core.App, since time.Time) (string, int, error) {
	bound := since.UTC().Format("2006-01-02 15:04:05.000Z")

	var rows []struct {
		SentBy string `db:"sent_by"`
		N      int    `db:"n"`
	}
	err := app.DB().NewQuery(`
		SELECT sent_by, COUNT(*) AS n
		FROM mail_messages
		WHERE (delivery_status = 'bounced' OR delivery_status = 'spam_complaint')
		  AND date >= {:since}
		  AND sent_by != ''
		GROUP BY sent_by
		ORDER BY n DESC
	`).Bind(map[string]any{"since": bound}).All(&rows)
	if err != nil {
		return "", 0, fmt.Errorf("group failures by sender: %w", err)
	}
	if len(rows) == 0 {
		return "", 0, nil
	}

	total := 0
	for _, r := range rows {
		total += r.N
	}
	if total < minFailuresToAttribute {
		return "", 0, nil
	}

	top := rows[0]
	if float64(top.N)/float64(total) < senderConcentration {
		return "", 0, nil
	}
	return top.SentBy, top.N, nil
}

// ContainSender disables one account and tells the administrators why.
//
// Disabling rather than throttling, because there is no per-account sending
// limit to reach for and because a compromised account should not keep a
// reduced budget — it should have none. Core enforces `disabled` at both
// authentication and refresh, so an existing session dies with it.
//
// The administrators are told because somebody has to restore the account,
// and because "why can Dana not log in" should have an answer waiting rather
// than becoming an investigation.
func ContainSender(app core.App, userID string, failures int, since time.Time) error {
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		return fmt.Errorf("find the sender: %w", err)
	}
	if user.GetBool("disabled") {
		// Already contained. Re-notifying every sweep would train the
		// administrators to ignore the notice.
		return nil
	}

	user.Set("disabled", true)
	if err := app.Save(user); err != nil {
		return fmt.Errorf("disable the sender: %w", err)
	}

	gateLog.Error("disabled an account after concentrated delivery failures",
		"user", userID, "failures", failures, "since", since)

	if _, err := notify.Administrators(app, notify.NotifyParams{
		Type:  "mail.sender_contained",
		Title: "An account has been suspended",
		Body: fmt.Sprintf(
			"Most recent delivery failures from this deployment (%d of them) came from a "+
				"single account, %s, so that account has been signed out and suspended "+
				"to protect mail delivery for everyone else.\n\n"+
				"This usually means the account's password has been obtained by somebody "+
				"else. Review its recent sent mail, reset its password, then re-enable it "+
				"from the people settings.",
			failures, user.GetString("email")),
		URL: "/settings/people",
	}); err != nil {
		// The account is already contained; failing to announce it must not
		// undo that.
		gateLog.Warn("could not tell the administrators about a contained account",
			"user", userID, "err", err)
	}
	return nil
}
