package mail

import (
	"net/http"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/rlstest"
)

// owner_claim_test.go proves that a logged-in user cannot make themselves the
// owner of a shared mailbox that has no members, against the SHIPPED
// migrations (rlstest).
//
// The mail_mailbox_members createRule has a bootstrap branch so that the user
// who creates a shared mailbox can add themselves as its first owner. Before
// 1830000007 that branch checked only "the mailbox has no members", so any
// non-guest could claim a mailbox whose last member had deleted their account
// and read all of its mail. The branch now also requires that the caller is
// the mailbox's created_by, which registerMailboxCreatorGuard pins.
//
// Each test sends one API request per app: the vendored PocketBase binds a
// new OnServe handler on every tests.ApiScenario run, so a second scenario on
// the same app panics with a duplicate /_/extensions.js route.

func memberBody(mailboxID, userID, role string) string {
	return `{"mailbox":"` + mailboxID + `","user":"` + userID + `","role":"` + role + `"}`
}

func mailboxBody(domainID, createdBy string) string {
	body := `{"address":"ops","domain":"` + domainID + `","name":"Ops","type":"shared"`
	if createdBy != "" {
		body += `,"created_by":"` + createdBy + `"`
	}
	return body + `}`
}

// newCreatedMailbox saves a shared mailbox the way the API leaves one right
// after MailboxForm's create call: created_by is the creator, and there is no
// member row yet. The API step itself is covered by the Create* tests below.
func newCreatedMailbox(t *testing.T, env *anonGuardEnv, creator *core.Record) *core.Record {
	t.Helper()
	mailboxes, err := env.app.FindCollectionByNameOrId("mail_mailboxes")
	if err != nil {
		t.Fatal(err)
	}
	mailbox := core.NewRecord(mailboxes)
	mailbox.Set("address", "ops")
	mailbox.Set("name", "Ops")
	mailbox.Set("domain", env.mailbox.GetString("domain"))
	mailbox.Set("type", "shared")
	mailbox.Set("created_by", creator.Id)
	if err := env.app.Save(mailbox); err != nil {
		t.Fatalf("save created mailbox: %v", err)
	}
	return mailbox
}

func TestOwnerClaim_OutsiderCannotClaimMemberlessMailbox(t *testing.T) {
	env := setupAnonGuardApp(t)
	anonCall{
		method: http.MethodPost,
		url:    listURL("mail_mailbox_members"),
		body:   memberBody(env.orphanMailbox.Id, env.outsider.Id, "owner"),
		status: http.StatusBadRequest,
	}.run(t, env, env.outsiderToken)

	n, err := env.app.CountRecords("mail_mailbox_members",
		dbx.HashExp{"mailbox": env.orphanMailbox.Id})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("orphan mailbox gained %d member rows; want 0", n)
	}
}

// The mailbox's creator is gone, so nobody can use the bootstrap branch; an
// operator repairs this state, not whichever user asks first.
func TestOwnerClaim_DeletedCreatorLeavesNoBootstrap(t *testing.T) {
	env := setupAnonGuardApp(t)
	leaver := mailGuestUser(t, env.app, "leaver@test.local", "member")
	env.orphanMailbox.Set("created_by", leaver.Id)
	if err := env.app.Save(env.orphanMailbox); err != nil {
		t.Fatal(err)
	}
	if err := env.app.Delete(leaver); err != nil {
		t.Fatalf("delete creator: %v", err)
	}

	mailbox, err := env.app.FindRecordById("mail_mailboxes", env.orphanMailbox.Id)
	if err != nil {
		t.Fatalf("mailbox must survive its creator's deletion: %v", err)
	}
	if got := mailbox.GetString("created_by"); got != "" {
		t.Fatalf("created_by = %q after the creator was deleted; want empty", got)
	}

	anonCall{
		method: http.MethodPost,
		url:    listURL("mail_mailbox_members"),
		body:   memberBody(env.orphanMailbox.Id, env.outsider.Id, "owner"),
		status: http.StatusBadRequest,
	}.run(t, env, env.outsiderToken)
}

// The second step of MailboxForm's create flow: the creator adds themselves
// as the first owner.
func TestOwnerClaim_CreatorBootstrapsNewSharedMailbox(t *testing.T) {
	env := setupAnonGuardApp(t)
	created := newCreatedMailbox(t, env, env.owner)
	anonCall{
		method:   http.MethodPost,
		url:      listURL("mail_mailbox_members"),
		body:     memberBody(created.Id, env.owner.Id, "owner"),
		status:   http.StatusOK,
		expected: []string{`"role":"owner"`},
	}.run(t, env, env.ownerToken)
}

// Another user cannot take a new mailbox in the gap before its creator adds
// their own membership.
func TestOwnerClaim_OtherUserCannotBootstrapNewMailbox(t *testing.T) {
	env := setupAnonGuardApp(t)
	created := newCreatedMailbox(t, env, env.owner)
	anonCall{
		method: http.MethodPost,
		url:    listURL("mail_mailbox_members"),
		body:   memberBody(created.Id, env.outsider.Id, "owner"),
		status: http.StatusBadRequest,
	}.run(t, env, env.outsiderToken)
}

// The creator cannot use the bootstrap branch to make someone else the
// owner: the new row must name the caller.
func TestOwnerClaim_CreatorCannotBootstrapSomeoneElse(t *testing.T) {
	env := setupAnonGuardApp(t)
	created := newCreatedMailbox(t, env, env.owner)
	anonCall{
		method: http.MethodPost,
		url:    listURL("mail_mailbox_members"),
		body:   memberBody(created.Id, env.outsider.Id, "owner"),
		status: http.StatusBadRequest,
	}.run(t, env, env.ownerToken)
}

// Guests never own mail infrastructure (1830000003), even a mailbox whose
// created_by names them.
func TestOwnerClaim_GuestCreatorCannotBootstrap(t *testing.T) {
	env := setupAnonGuardApp(t)
	guest := mailGuestUser(t, env.app, "guest@test.local", "guest")
	token, err := guest.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	created := newCreatedMailbox(t, env, guest)
	anonCall{
		method: http.MethodPost,
		url:    listURL("mail_mailbox_members"),
		body:   memberBody(created.Id, guest.Id, "owner"),
		status: http.StatusBadRequest,
	}.run(t, env, token)
}

// With an owner in place, the creator adds others through the owner branch
// like any owner.
func TestOwnerClaim_CreatorAddsMembersAfterBootstrap(t *testing.T) {
	env := setupAnonGuardApp(t)
	created := newCreatedMailbox(t, env, env.owner)
	addMailboxMember(t, env.app, created, env.owner, "owner")
	anonCall{
		method:   http.MethodPost,
		url:      listURL("mail_mailbox_members"),
		body:     memberBody(created.Id, env.teammate.Id, "member"),
		status:   http.StatusOK,
		expected: []string{`"role":"member"`},
	}.run(t, env, env.ownerToken)
}

// A client cannot choose created_by: the API records the caller, whatever
// the body says.
func TestOwnerClaim_CreateRecordsTheCallerAsCreator(t *testing.T) {
	env := setupAnonGuardApp(t)
	registerMailboxCreatorGuard(env.app)
	anonCall{
		method:   http.MethodPost,
		url:      listURL("mail_mailboxes"),
		body:     mailboxBody(env.mailbox.GetString("domain"), env.outsider.Id),
		status:   http.StatusOK,
		expected: []string{`"created_by":"` + env.owner.Id + `"`},
	}.run(t, env, env.ownerToken)
}

// Released clients send no created_by; their create still works and still
// records the caller.
func TestOwnerClaim_CreateWithoutCreatedByRecordsTheCaller(t *testing.T) {
	env := setupAnonGuardApp(t)
	registerMailboxCreatorGuard(env.app)
	anonCall{
		method:   http.MethodPost,
		url:      listURL("mail_mailboxes"),
		body:     mailboxBody(env.mailbox.GetString("domain"), ""),
		status:   http.StatusOK,
		expected: []string{`"created_by":"` + env.owner.Id + `"`},
	}.run(t, env, env.ownerToken)
}

// An owner may still edit the mailbox, but cannot name someone else as its
// creator: that would hand the bootstrap right to a user of their choice
// once the mailbox empties.
func TestOwnerClaim_OwnerCannotRewriteCreatedBy(t *testing.T) {
	env := setupAnonGuardApp(t)
	registerMailboxCreatorGuard(env.app)
	env.mailbox.Set("created_by", env.owner.Id)
	if err := env.app.Save(env.mailbox); err != nil {
		t.Fatal(err)
	}
	anonCall{
		method: http.MethodPatch,
		url:    recordURL("mail_mailboxes", env.mailbox.Id),
		body:   `{"display_name":"Support desk","created_by":"` + env.outsider.Id + `"}`,
		status: http.StatusOK,
		expected: []string{
			`"display_name":"Support desk"`,
			`"created_by":"` + env.owner.Id + `"`,
		},
	}.run(t, env, env.ownerToken)
}

func TestOwnerClaim_ShippedRulesPinTheCreator(t *testing.T) {
	env := setupMailShareApp(t)
	rlstest.RequireRuleContains(t, env.app, "mail_mailbox_members", "create",
		`user = @request.auth.id && role = "owner" && mailbox.mail_mailbox_members_via_mailbox.id = "" && mailbox.created_by = @request.auth.id && @request.auth.role != "guest"`)

	col, err := env.app.FindCollectionByNameOrId("mail_mailboxes")
	if err != nil {
		t.Fatal(err)
	}
	field, ok := col.Fields.GetByName("created_by").(*core.RelationField)
	if !ok {
		t.Fatal("mail_mailboxes.created_by must be a relation field")
	}
	// Required would block the creator's account delete, and cascade would
	// delete the mailbox and its mail with it; the field must simply clear.
	if field.Required || field.CascadeDelete {
		t.Fatalf("created_by required=%v cascadeDelete=%v; want both false",
			field.Required, field.CascadeDelete)
	}
}
