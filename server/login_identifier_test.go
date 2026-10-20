package mail

import "testing"

func TestLoginIdentifier(t *testing.T) {
	t.Parallel()
	app := newAddressTestApp(t)
	org := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.org", "verified": true})
	com := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.com", "verified": true})
	user := saveRealRecord(t, app, "users", map[string]any{
		"email": "nathan@argosity.com", "username": "nathan", "name": "Nathan", "role": "owner",
	})
	inbox := saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "nathan", "domain": org.Id, "type": "personal", "display_name": "Nathan", "name": "Nathan",
	})
	saveRealRecord(t, app, "mail_identities", map[string]any{"user": user.Id, "inbox": inbox.Id, "email_name": "nathan"})
	saveRealRecord(t, app, "mail_addresses", map[string]any{"local_part": "nathan", "domain": com.Id, "mailbox": inbox.Id, "source": addressSourceManual})
	saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "support", "domain": org.Id, "type": "shared", "display_name": "Support", "name": "Support",
	})

	for _, tc := range []struct{ in, want string }{
		{"nathan", "nathan"},                           // bare username: unchanged
		{"nathan@argosity.com", "nathan@argosity.com"}, // account email: unchanged
		{"nathan@tinycld.org", "nathan"},               // primary address
		{"Nathan@TinyCld.com", "nathan"},               // added address, any case
		{"support@tinycld.org", "support@tinycld.org"}, // shared inbox: no person to log in as
		{"nobody@tinycld.org", "nobody@tinycld.org"},   // unknown: unchanged, fails later
	} {
		if got := loginIdentifier(app, tc.in); got != tc.want {
			t.Fatalf("loginIdentifier(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
