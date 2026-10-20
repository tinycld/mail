package mail

import (
	"context"
	"testing"

	"tinycld.org/core/syscfg"
	"tinycld.org/packages/mail/api"
)

// Sending as an address on a second domain signs and stores as that domain.
func TestSendMessage_FromAddressOnSecondDomain(t *testing.T) {
	env := setupSendAddressEnv(t)
	syscfg.SetResolver(func(key string) string {
		if key == "mail.postmark_server_token" {
			return "test-token"
		}
		return ""
	})
	t.Cleanup(func() { syscfg.SetResolver(func(string) string { return "" }) })

	env.com.Set("spf_verified", true)
	env.com.Set("dkim_verified", true)
	env.com.Set("return_path_verified", true)
	if err := env.app.Save(env.com); err != nil {
		t.Fatal(err)
	}
	user := saveRealRecord(t, env.app, "users", map[string]any{
		"email": "n@ext.example", "username": "nathan", "name": "Nathan", "role": "owner", "is_demo": true,
	})
	saveRealRecord(t, env.app, "mail_mailbox_members", map[string]any{"mailbox": env.inbox.Id, "user": user.Id, "role": "owner"})
	com, err := findAddressRow(env.app, "nathan", env.com.Id)
	if err != nil {
		t.Fatal(err)
	}

	result, err := sendMessage(env.app, sendParams{
		Ctx:           context.Background(),
		UserID:        user.Id,
		MailboxID:     env.inbox.Id,
		FromAddressID: com.Id,
		To:            []api.Recipient{{Email: "friend@example.net"}},
		Subject:       "hello",
		TextBody:      "hi",
	})
	if err != nil {
		t.Fatal(err)
	}

	stored, err := env.app.FindRecordById("mail_messages", result.MessageRecordID)
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

func TestVerifyAliasBelongsToMailbox_RejectsMismatch(t *testing.T) {
	t.Parallel()
	app := setupAliasTestApp(t)

	seedDomainAndMailbox(t, app, "acme.com", "support", "mb_send_test005")
	seedDomainAndMailbox(t, app, "acme2.com", "sales", "mb_send_test006")
	seedAlias(t, app, "mb_send_test006", "help", "al_send_test006")

	alias, err := app.FindRecordById("mail_mailbox_aliases", padID("al_send_test006"))
	if err != nil {
		t.Fatalf("failed to load alias: %v", err)
	}

	if verifyAliasBelongsToMailbox(alias, padID("mb_send_test005")) == nil {
		t.Fatal("expected error for alias belonging to different mailbox")
	}
	if err := verifyAliasBelongsToMailbox(alias, padID("mb_send_test006")); err != nil {
		t.Fatalf("expected no error for matching mailbox, got %v", err)
	}
	if err := verifyAliasBelongsToMailbox(nil, padID("mb_send_test005")); err != nil {
		t.Fatalf("expected no error for nil alias, got %v", err)
	}
}
