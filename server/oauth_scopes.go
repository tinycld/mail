package mail

import "tinycld.org/core/oauth"

// Mail's OAuth scopes. mail:read governs everything a token may read;
// mail:send is the write scope — its name says what a write means here.
const (
	scopeRead  = "mail:read"
	scopeWrite = "mail:send"
)

// oauthPackage declares what an OAuth token may reach in mail. Registered
// from registerShared; the catalog, the consent screen and the CLI's login
// request are all derived from it. A route or collection missing here is
// default-denied for OAuth callers only — sessions still work — so the CLI's
// surface is pinned in oauth_scopes_test.go.
func oauthPackage() oauth.Package {
	rw := oauth.Access{Read: []string{scopeRead}, Write: []string{scopeWrite}}
	ro := oauth.Access{Read: []string{scopeRead}}
	return oauth.Package{
		Slug: "mail",
		Scopes: []oauth.Scope{
			{ID: scopeRead, Label: "Read your email"},
			{ID: scopeWrite, Label: "Send email on your behalf"},
		},
		Collections: map[string]oauth.Access{
			"mail_messages":     rw,
			"mail_threads":      rw,
			"mail_thread_state": rw,
			"mail_mailboxes":    rw,

			// Read-only surfaces the CLI needs: per-folder unread counts (a
			// view), the caller's mailbox memberships, and the aliases a send
			// can pick a From identity from. Aliases are administered in the
			// app, so no write.
			//
			// mail_domains is here for a non-obvious reason: mail_mailboxes.
			// address stores only the LOCAL PART, so every full address the
			// CLI prints or matches on has to join the domain row. Without it
			// `mail mailboxes`, `mail send` and `--mailbox <address>` all fail
			// closed on the domain read. Domains are administered in the app,
			// so no write.
			"mail_folder_counts":   ro,
			"mail_mailbox_members": ro,
			"mail_mailbox_aliases": ro,
			"mail_domains":         ro,

			// Labels are CORE collections shared with contacts, which claims
			// them too; core unions the two, so either package's grant reaches
			// them — requiring both would make labelling mail impossible for
			// a mail-only grant.
			"labels":            rw,
			"label_assignments": rw,
		},
		Endpoints: map[string][]string{
			"GET /api/mail/search": {scopeRead},
			"POST /api/mail/send":  {scopeWrite},
			"POST /api/mail/draft": {scopeWrite},
		},
	}
}
