package mail

import (
	"strings"
	"testing"

	"tinycld.org/core/syscfg"
)

func smtpSendEnv(t *testing.T) (*sendAddressEnv, *smtpSession) {
	t.Helper()
	env := setupSendAddressEnv(t)
	syscfg.SetResolver(func(key string) string {
		if key == "mail.postmark_server_token" {
			return "test-token"
		}
		return ""
	})
	t.Cleanup(func() { syscfg.SetResolver(func(string) string { return "" }) })

	for _, d := range []string{"tinycld.org", "tinycld.com"} {
		domain, err := env.app.FindFirstRecordByData("mail_domains", "domain", d)
		if err != nil {
			t.Fatal(err)
		}
		domain.Set("spf_verified", true)
		domain.Set("dkim_verified", true)
		domain.Set("return_path_verified", true)
		if err := env.app.Save(domain); err != nil {
			t.Fatal(err)
		}
	}
	user := saveRealRecord(t, env.app, "users", map[string]any{
		"email": "n@ext.example", "username": "nathan", "name": "Nathan", "role": "owner", "is_demo": true,
	})
	saveRealRecord(t, env.app, "mail_mailbox_members", map[string]any{"mailbox": env.inbox.Id, "user": user.Id, "role": "owner"})
	return env, &smtpSession{app: env.app, user: user}
}

func TestSMTPSubmission_SendsAsSecondDomainAddress(t *testing.T) {
	env, s := smtpSendEnv(t)
	com, _ := findAddressRow(env.app, "nathan", env.com.Id)

	if err := s.Mail("nathan@tinycld.com", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Rcpt("friend@example.net", nil); err != nil {
		t.Fatal(err)
	}
	raw := "From: nathan@tinycld.com\r\nTo: friend@example.net\r\nSubject: hi\r\nMessage-ID: <sub1@tinycld.com>\r\n\r\nbody\r\n"
	if err := s.Data(strings.NewReader(raw)); err != nil {
		t.Fatal(err)
	}

	stored, err := env.app.FindFirstRecordByData("mail_messages", "subject", "hi")
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.GetString("sender_email"); got != "nathan@tinycld.com" {
		t.Fatalf("sender_email = %q, want nathan@tinycld.com", got)
	}
	if got := stored.GetString("sent_from"); got != com.Id {
		t.Fatalf("sent_from = %q, want %q", got, com.Id)
	}
}

func TestSMTPSubmission_RefusesAnotherInboxsAddress(t *testing.T) {
	env, s := smtpSendEnv(t)
	saveRealRecord(t, env.app, "mail_mailboxes", map[string]any{
		"address": "support", "domain": env.org.Id, "type": "shared", "display_name": "Support", "name": "Support",
	})
	logs := captureLogs(t)

	err := s.Mail("support@tinycld.org", nil)
	if err == nil || !strings.Contains(err.Error(), "Not authorized") {
		t.Fatalf("want a Not authorized refusal, got %v", err)
	}
	if !strings.Contains(logs.String(), "support@tinycld.org") {
		t.Fatalf("refusal not logged:\n%s", logs.String())
	}
}
