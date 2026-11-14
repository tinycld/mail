package mail

import (
	"context"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/addressbook"
	"tinycld.org/core/automation"
)

func registerClassifiedTrigger() {
	automation.RegisterTriggerFilter("mail:message-classified", classificationIsMail)
	automation.RegisterOwnerResolver("mail:message-classified", classificationOwner)
	automation.RegisterFieldsResolver("mail:message-classified", classifiedFields)
	automation.RegisterFieldsResolver("mail:message-received", receivedFields)
	for _, ref := range []string{"mail:message-received", "mail:message-classified"} {
		automation.RegisterExtraFieldFilter(ref, offerExtraField)
	}
}

// offerExtraField hides "sender is in my contacts" where no address book is
// installed: the fact is absent there, so a rule on it could never match.
func offerExtraField(_ core.App, key string) bool {
	return key != "sender_in_contacts" || addressbook.Available()
}

// classifiedSendGuard applies mail's automatic-mail guard to the message a
// classification is about; core looks guards up by the trigger record's
// collection, which here is classifications.
func classifiedSendGuard(app core.App, rec *core.Record) (bool, string) {
	if rec.GetString("collection") != "mail_messages" {
		return false, ""
	}
	msg, err := messageForRecord(app, rec)
	if err != nil {
		// Failing open would let a send bypass the automatic-mail loop guard.
		return true, "message not found"
	}
	return automaticMailSendGuard(app, msg)
}

// classificationIsMail lets only real mail decisions fire rules: a preview
// changed nothing, and an error row has no label to act on.
func classificationIsMail(_ core.App, rec *core.Record) bool {
	return rec != nil && rec.GetString("collection") == "mail_messages" &&
		!rec.GetBool("preview") && rec.GetString("error") == "" && rec.GetString("label_name") != ""
}

func classificationOwner(_ core.App, rec *core.Record) []string {
	if u := rec.GetString("user"); u != "" {
		return []string{u}
	}
	return nil
}

// messageForRecord is the message a mail action works on: the trigger record
// itself, or the message a classification row is about.
func messageForRecord(app core.App, rec *core.Record) (*core.Record, error) {
	if rec == nil {
		return nil, fmt.Errorf("no trigger record")
	}
	if rec.Collection().Name != "classifications" {
		return rec, nil
	}
	return app.FindRecordById("mail_messages", rec.GetString("record_id"))
}

func factFields(app core.App, userID, senderEmail string) []automation.DynamicField {
	f := factsFor(context.Background(), app, userID, senderEmail)
	out := []automation.DynamicField{{Key: "has_sent_to_sender", Label: "I have sent email to the sender", Type: "boolean", Value: f.HasSentTo}}
	if f.InContacts != nil {
		out = append(out, automation.DynamicField{Key: "sender_in_contacts", Label: "Sender is in my contacts", Type: "boolean", Value: *f.InContacts})
	}
	return out
}

// receivedFields offers the facts only to personal rules: an org rule acts for
// every mailbox member, and "my contacts" has no single answer there.
func receivedFields(app core.App, owner string, _ map[string]any, rec *core.Record) []automation.DynamicField {
	if owner == "" {
		return nil
	}
	return factFields(app, owner, rec.GetString("sender_email"))
}

func classifiedFields(app core.App, _ string, _ map[string]any, rec *core.Record) []automation.DynamicField {
	msg, err := messageForRecord(app, rec)
	if err != nil {
		return nil
	}
	out := []automation.DynamicField{
		{Key: "subject", Label: "Subject", Type: "text", Value: msg.GetString("subject")},
		{Key: "sender_email", Label: "Sender", Type: "text", Value: msg.GetString("sender_email")},
		{Key: "sender_name", Label: "Sender name", Type: "text", Value: msg.GetString("sender_name")},
		{Key: "archived", Label: "Archived by AI", Type: "boolean", Value: rec.GetString("action") == actionLabelArchive},
	}
	return append(out, factFields(app, rec.GetString("user"), msg.GetString("sender_email"))...)
}
