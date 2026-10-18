package mail

import (
	"slices"
	"strings"

	gomail "github.com/emersion/go-message/mail"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/automation"
)

// isAutoSubmitted reports whether a message's headers mark it as sent
// automatically: Auto-Submitted with any value but "no" (RFC 3834), our own
// loop header, or Precedence bulk, list or junk. Every inbound path reads the
// flag through this one function, so they all agree on what is automatic.
func isAutoSubmitted(headers []Header) bool {
	for _, h := range headers {
		value := strings.ToLower(strings.TrimSpace(h.Value))
		switch {
		case strings.EqualFold(h.Name, automation.AutoSubmittedHeader):
			// A value can carry parameters ("auto-replied; owner-email=…").
			keyword, _, _ := strings.Cut(value, ";")
			if strings.TrimSpace(keyword) != "no" {
				return true
			}
		case strings.EqualFold(h.Name, automation.LoopHeader):
			return true
		case strings.EqualFold(h.Name, "Precedence"):
			if value == "bulk" || value == "list" || value == "junk" {
				return true
			}
		}
	}
	return false
}

// rfc5322Headers lists a parsed message's top-level header fields, in the
// shape the webhook payloads give them.
func rfc5322Headers(h gomail.Header) []Header {
	var headers []Header
	fields := h.Fields()
	for fields.Next() {
		headers = append(headers, Header{Name: fields.Key(), Value: fields.Value()})
	}
	return headers
}

// ruleSendHeaders are core's rule-mail headers (automation.RuleMailHeaders)
// as a header list, sorted by name so a send's headers have a fixed order.
func ruleSendHeaders(app core.App, autoSubmitted string) []Header {
	byName := automation.RuleMailHeaders(app, autoSubmitted)
	headers := make([]Header, 0, len(byName))
	for name, value := range byName {
		headers = append(headers, Header{Name: name, Value: value})
	}
	slices.SortFunc(headers, func(a, b Header) int { return strings.Compare(a.Name, b.Name) })
	return headers
}

// automaticMailReason is what run history shows for a rule send skipped
// because the trigger message was sent automatically.
const automaticMailReason = "The message was sent automatically."

// automaticMailSendGuard is mail's send guard for mail_messages: core's rule
// actions that send mail (core:send-email) skip a message that was sent
// automatically, the same as mail's own send actions do.
func automaticMailSendGuard(_ core.App, message *core.Record) (bool, string) {
	if !message.GetBool("is_auto_submitted") {
		return false, ""
	}
	return true, automaticMailReason
}

// skipAutomaticMail returns a skip when a send action must not run on the
// trigger message, because the message was sent automatically: answering it
// could start a loop with the other sender. The run history shows the
// action as "Skipped" with the reason.
func skipAutomaticMail(message *core.Record) error {
	if message == nil || !message.GetBool("is_auto_submitted") {
		return nil
	}
	return automation.Skip(automaticMailReason)
}
