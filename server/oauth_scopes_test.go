package mail

import (
	"testing"

	"tinycld.org/core/oauth"
)

// Every route the mail CLI drives must resolve to a scope. An unclassified
// route 403s for OAuth callers only — sessions still work — so the CLI's fake
// server never notices; this is the test that does.
func TestOAuthClassifiesCLIRoutes(t *testing.T) {
	oauth.RegisterPackage(oauthPackage())

	for _, r := range []struct{ method, path, scope string }{
		{"GET", "/api/mail/search", scopeRead},
		{"POST", "/api/mail/send", scopeWrite},
		{"POST", "/api/mail/draft", scopeWrite},
		{"GET", "/api/collections/mail_messages/records", scopeRead},
		{"POST", "/api/collections/mail_threads/records", scopeWrite},
		// `mail read` fetches the body_html file the record points at.
		{"GET", "/api/files/mail_messages/rec123/body_ab12cd34ef.html", scopeRead},
		// `mail status` reads the folder-counts view; send/list resolve the
		// caller's mailbox memberships, `--from` identities and the domain
		// row every full address joins.
		{"GET", "/api/collections/mail_folder_counts/records", scopeRead},
		{"GET", "/api/collections/mail_mailbox_members/records", scopeRead},
		{"GET", "/api/collections/mail_mailbox_aliases/records", scopeRead},
		{"GET", "/api/collections/mail_domains/records", scopeRead},
		// Label add/remove.
		{"GET", "/api/collections/labels/records", scopeRead},
		{"POST", "/api/collections/label_assignments/records", scopeWrite},
		{"DELETE", "/api/collections/label_assignments/records/abc123", scopeWrite},
	} {
		rule := oauth.ScopeForRoute(r.method, r.path)
		if len(rule) == 0 {
			t.Errorf("%s %s is default-denied for OAuth callers", r.method, r.path)
			continue
		}
		if !rule.SatisfiedBy([]string{r.scope}) {
			t.Errorf("%s %s: %q must admit it (got %v)", r.method, r.path, r.scope, rule)
		}
	}
}

// Aliases and domains are administered in the app; the CLI only reads them.
// A read grant must not have opened a write path to DNS/verification state.
func TestOAuthAdministeredCollectionsStayReadOnly(t *testing.T) {
	oauth.RegisterPackage(oauthPackage())

	for _, c := range []string{"mail_folder_counts", "mail_mailbox_members", "mail_mailbox_aliases", "mail_domains"} {
		for _, m := range []string{"POST", "PATCH", "DELETE"} {
			p := "/api/collections/" + c + "/records"
			if m != "POST" {
				p += "/abc"
			}
			if got := oauth.ScopeForRoute(m, p); got.SatisfiedBy([]string{scopeRead, scopeWrite}) {
				t.Errorf("%s %s must stay denied, got %v", m, p, got)
			}
		}
	}
	if oauth.ScopeForRoute("POST", "/api/mail/send").SatisfiedBy([]string{scopeRead}) {
		t.Error("mail:read alone must not admit a send")
	}
}

// The search source's scopes must be scopes this package actually registers,
// or the federated search would admit a scope no grant can carry.
func TestSearchSourceScopesAreRegistered(t *testing.T) {
	oauth.RegisterPackage(oauthPackage())
	registered := oauth.PackageScopes("mail")
	for _, s := range searchSource().Scopes {
		if !oauth.HasScope(registered, s) {
			t.Errorf("search source names scope %q, which mail does not register (%v)", s, registered)
		}
	}
}
