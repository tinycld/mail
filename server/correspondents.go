package mail

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// recordCorrespondents remembers every address a sent message went to, for
// the user who sent it. Called on create and update of mail_messages: a send
// may be stored first and marked sent afterwards.
func recordCorrespondentsOnUpdate(app core.App, msg *core.Record) error {
	// Flag updates (read, starred, labels) fire this hook for every sent
	// message; only the move to "sent" or a change of sender adds recipients.
	orig := msg.Original()
	if orig.GetString("delivery_status") == msg.GetString("delivery_status") &&
		orig.GetString("sent_by") == msg.GetString("sent_by") {
		return nil
	}
	return recordCorrespondents(app, msg)
}

func recordCorrespondents(app core.App, msg *core.Record) error {
	userID := msg.GetString("sent_by")
	if userID == "" || msg.GetString("delivery_status") != "sent" {
		return nil
	}
	col, err := app.FindCachedCollectionByNameOrId("mail_correspondents")
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, field := range []string{"recipients_to", "recipients_cc", "recipients_bcc"} {
		var recips []Recipient
		raw, _ := json.Marshal(msg.Get(field))
		var s string
		if json.Unmarshal(raw, &s) == nil {
			raw = []byte(s)
		}
		if json.Unmarshal(raw, &recips) != nil {
			continue
		}
		for _, r := range recips {
			email := strings.ToLower(strings.TrimSpace(r.Email))
			if email == "" {
				continue
			}
			rec, ferr := app.FindFirstRecordByFilter("mail_correspondents", "user = {:u} && email = {:e}", map[string]any{"u": userID, "e": email})
			if ferr != nil {
				rec = core.NewRecord(col)
				rec.Set("user", userID)
				rec.Set("email", email)
			}
			rec.Set("last_sent", now)
			if err := app.Save(rec); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasSentTo(app core.App, userID, email string) bool {
	_, err := app.FindFirstRecordByFilter("mail_correspondents", "user = {:u} && email = {:e}",
		map[string]any{"u": userID, "e": strings.ToLower(strings.TrimSpace(email))})
	return err == nil
}
