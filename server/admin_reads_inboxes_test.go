package mail

import (
	"net/http"
	"testing"
)

// admin_reads_inboxes_test.go proves 2040000012 against the shipped
// migrations: an admin or owner reads every shared inbox and its member rows,
// and nobody gains access to the mail itself.

type adminReadEnv struct {
	*anonGuardEnv
	adminToken     string
	roleOwnerToken string
	guestToken     string
}

func setupAdminReadApp(t *testing.T) *adminReadEnv {
	t.Helper()
	env := setupAnonGuardApp(t)
	admin := mailGuestUser(t, env.app, "reader-admin@test.local", "admin")
	owner := mailGuestUser(t, env.app, "reader-owner@test.local", "owner")
	guest := mailGuestUser(t, env.app, "reader-guest@test.local", "guest")
	tokens := map[string]string{}
	for name, user := range map[string]interface{ NewAuthToken() (string, error) }{
		"admin": admin, "owner": owner, "guest": guest,
	} {
		token, err := user.NewAuthToken()
		if err != nil {
			t.Fatal(err)
		}
		tokens[name] = token
	}
	return &adminReadEnv{
		anonGuardEnv: env, adminToken: tokens["admin"],
		roleOwnerToken: tokens["owner"], guestToken: tokens["guest"],
	}
}

func TestAdminReads_AdminListsInboxAndMembers(t *testing.T) {
	env := setupAdminReadApp(t)
	anonCall{
		method: http.MethodGet, url: listURL("mail_mailboxes"), status: http.StatusOK,
		expected: []string{`"totalItems":2`, env.mailbox.Id, env.orphanMailbox.Id},
	}.run(t, env.anonGuardEnv, env.adminToken)
	anonCall{
		method: http.MethodGet, url: listURL("mail_mailbox_members"), status: http.StatusOK,
		expected: []string{`"totalItems":1`, env.owner.Id},
	}.run(t, env.anonGuardEnv, env.adminToken)
	anonCall{
		method: http.MethodGet, url: recordURL("mail_mailboxes", env.mailbox.Id),
		status: http.StatusOK, expected: []string{env.mailbox.Id},
	}.run(t, env.anonGuardEnv, env.adminToken)
}

func TestAdminReads_OwnerRoleListsInboxAndMembers(t *testing.T) {
	env := setupAdminReadApp(t)
	anonCall{
		method: http.MethodGet, url: listURL("mail_mailboxes"), status: http.StatusOK,
		expected: []string{`"totalItems":2`, env.mailbox.Id},
	}.run(t, env.anonGuardEnv, env.roleOwnerToken)
	anonCall{
		method: http.MethodGet, url: listURL("mail_mailbox_members"), status: http.StatusOK,
		expected: []string{`"totalItems":1`, env.owner.Id},
	}.run(t, env.anonGuardEnv, env.roleOwnerToken)
}

func TestAdminReads_PlainNonMemberSeesNothing(t *testing.T) {
	env := setupAdminReadApp(t)
	for _, collection := range []string{"mail_mailboxes", "mail_mailbox_members"} {
		anonCall{
			method: http.MethodGet, url: listURL(collection), status: http.StatusOK,
			expected: []string{`"totalItems":0`},
		}.run(t, env.anonGuardEnv, env.outsiderToken)
	}
}

func TestAdminReads_GuestSeesNothing(t *testing.T) {
	env := setupAdminReadApp(t)
	for _, collection := range []string{"mail_mailboxes", "mail_mailbox_members"} {
		anonCall{
			method: http.MethodGet, url: listURL(collection), status: http.StatusOK,
			expected: []string{`"totalItems":0`},
		}.run(t, env.anonGuardEnv, env.guestToken)
	}
}

func TestAdminReads_AnonymousSeesNothing(t *testing.T) {
	env := setupAdminReadApp(t)
	for _, collection := range []string{"mail_mailboxes", "mail_mailbox_members"} {
		anonCall{
			method: http.MethodGet, url: listURL(collection), status: http.StatusOK,
			expected: []string{`"totalItems":0`},
		}.run(t, env.anonGuardEnv, "")
	}
}

func TestAdminReads_AdminStillCannotReadMail(t *testing.T) {
	env := setupAdminReadApp(t)
	for _, token := range []string{env.adminToken, env.roleOwnerToken} {
		anonCall{
			method: http.MethodGet, url: listURL("mail_threads"), status: http.StatusOK,
			expected: []string{`"totalItems":0`},
			absent:   []string{"member thread", "sales secrets"},
		}.run(t, env.anonGuardEnv, token)
		anonCall{
			method: http.MethodGet, url: listURL("mail_messages"), status: http.StatusOK,
			expected: []string{`"totalItems":0`},
			absent:   []string{"member thread", "sales secrets"},
		}.run(t, env.anonGuardEnv, token)
	}
}

func TestAdminReads_AdminCannotWriteInbox(t *testing.T) {
	env := setupAdminReadApp(t)
	anonCall{
		method: http.MethodPatch, url: recordURL("mail_mailboxes", env.mailbox.Id),
		body: `{"name":"pwned"}`, status: http.StatusNotFound,
	}.run(t, env.anonGuardEnv, env.adminToken)
}

// Ruling L: reading every inbox must not let an admin join one.
func TestAdminReads_AdminCannotAddThemselvesAsMember(t *testing.T) {
	env := setupAdminReadApp(t)
	admin, err := env.app.FindAuthRecordByEmail("users", "reader-admin@test.local")
	if err != nil {
		t.Fatal(err)
	}
	anonCall{
		method: http.MethodPost, url: listURL("mail_mailbox_members"),
		body:   `{"mailbox":"` + env.mailbox.Id + `","user":"` + admin.Id + `","role":"member"}`,
		status: http.StatusBadRequest,
	}.run(t, env.anonGuardEnv, env.adminToken)

	rows, err := env.app.FindRecordsByFilter("mail_mailbox_members", "user = {:u}", "", 0, 0, map[string]any{"u": admin.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("admin gained %d member rows", len(rows))
	}
	for _, collection := range []string{"mail_threads", "mail_messages"} {
		anonCall{
			method: http.MethodGet, url: listURL(collection), status: http.StatusOK,
			expected: []string{`"totalItems":0`},
			absent:   []string{"member thread", "sales secrets"},
		}.run(t, env.anonGuardEnv, env.adminToken)
	}
}
