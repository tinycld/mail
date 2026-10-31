package mail

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// fakePostmark stands in for Postmark's send API and records each email it
// is asked to send, so a test can read the From header and metadata that
// would have gone out.
func fakePostmark(t *testing.T, app core.App) *[]map[string]any {
	t.Helper()
	return fakePostmarkWith(t, app, nil)
}

// fakePostmarkWith is fakePostmark that also runs onSend, when not nil,
// inside each send before it answers.
func fakePostmarkWith(t *testing.T, app core.App, onSend func()) *[]map[string]any {
	t.Helper()
	var sent []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var email map[string]any
		if err := json.Unmarshal(body, &email); err != nil {
			t.Errorf("fake Postmark: bad body %s: %v", body, err)
		}
		sent = append(sent, email)
		if onSend != nil {
			onSend()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ErrorCode":0,"Message":"OK","MessageID":"pm-1"}`))
	}))
	t.Cleanup(server.Close)
	setTestSetting(app, "mail.postmark_api_url", server.URL)
	return &sent
}

func smtpSendEnv(t *testing.T) (*sendAddressEnv, *smtpSession) {
	t.Helper()
	env := setupSendAddressEnv(t)
	setTestSetting(env.app, "mail.postmark_server_token", "test-token")

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
	// A real (non-demo) send, so the message reaches the provider.
	s.user.Set("is_demo", false)
	if err := env.app.Save(s.user); err != nil {
		t.Fatal(err)
	}
	sent := fakePostmark(t, env.app)

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

	if len(*sent) != 1 {
		t.Fatalf("provider got %d sends, want 1", len(*sent))
	}
	email := (*sent)[0]
	if from, _ := email["From"].(string); !strings.HasSuffix(from, "<nathan@tinycld.com>") {
		t.Fatalf("outgoing From = %q, want the tinycld.com address", from)
	}
	metadata, _ := email["Metadata"].(map[string]any)
	if got := metadata[senderDomainMetadataKey]; got != "tinycld.com" {
		t.Fatalf("sender domain metadata = %v, want tinycld.com", got)
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
