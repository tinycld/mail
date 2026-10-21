package mail

import "testing"

func TestAddressOwner(t *testing.T) {
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

	for _, tc := range []struct {
		in     string
		wantID string
	}{
		{"nathan@tinycld.org", user.Id},      // primary address
		{"Nathan@TinyCld.com", user.Id},      // added address, any case
		{"nathan+news@tinycld.org", user.Id}, // plus tag
		{"nathan@argosity.com", ""},          // account email: not an address mail hosts
		{"support@tinycld.org", ""},          // shared inbox: no person to sign in as
		{"nobody@tinycld.org", ""},           // unknown
	} {
		got, err := addressOwner(app, tc.in)
		if err != nil {
			t.Fatalf("addressOwner(%q): %v", tc.in, err)
		}
		gotID := ""
		if got != nil {
			gotID = got.Id
		}
		if gotID != tc.wantID {
			t.Fatalf("addressOwner(%q) = %q, want %q", tc.in, gotID, tc.wantID)
		}
	}
}
