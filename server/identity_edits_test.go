package mail

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

type identityEditSeed struct {
	admin     *core.Record
	ann       *core.Record
	annID     *core.Record
	domain    *core.Record
	annInbox  *core.Record
	shared    *core.Record
	otherUser *core.Record
}

func newIdentityEditApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := newRuleTestApp(t)
	registerIdentityEditHooks(app)
	return app
}

func seedIdentityEdit(t *testing.T, app *tests.TestApp) *identityEditSeed {
	t.Helper()
	s := &identityEditSeed{}
	s.domain = saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org", "verified": true})
	s.admin = saveRealRecord(t, app, "users", map[string]any{"email": "admin@example.net", "username": "adminuser", "name": "Admin", "role": "admin"})
	s.ann = saveRealRecord(t, app, "users", map[string]any{"email": "ann@example.net", "username": "ann", "name": "Ann", "role": "member"})
	s.otherUser = saveRealRecord(t, app, "users", map[string]any{"email": "bob@example.net", "username": "bob", "name": "Bob", "role": "member"})
	s.annID = identityOf(t, app, s.ann.Id)
	if s.annID == nil {
		t.Fatal("seed: ann has no identity")
	}
	s.annInbox = reloaded(t, app, mustRecord(t, app, "mail_mailboxes", s.annID.GetString("inbox")))
	s.shared = saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "team", "domain": s.domain.Id, "type": "shared", "display_name": "Team", "name": "Team",
	})
	return s
}

func mustRecord(t *testing.T, app core.App, collection, id string) *core.Record {
	t.Helper()
	r, err := app.FindRecordById(collection, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func addressRowID(t *testing.T, app core.App, local string, domain *core.Record) string {
	t.Helper()
	row, err := findAddressRow(app, local, domain.Id)
	if err != nil || row == nil {
		t.Fatalf("no address row %s: %v", local, err)
	}
	return row.Id
}

type identityEditCase struct {
	name    string
	actor   func(s *identityEditSeed) *core.Record // nil actor: anonymous
	body    func(t *testing.T, app *tests.TestApp, s *identityEditSeed) string
	status  int
	content string
	prepare func(t *testing.T, app *tests.TestApp, s *identityEditSeed)
	after   func(t *testing.T, app *tests.TestApp, s *identityEditSeed)
}

func runIdentityEdit(t *testing.T, c identityEditCase) {
	t.Helper()
	var seed *identityEditSeed
	scenario := tests.ApiScenario{
		Name:            c.name,
		Method:          http.MethodPatch,
		ExpectedStatus:  c.status,
		ExpectedContent: []string{c.content},
		Headers:         map[string]string{"Content-Type": "application/json"},
	}
	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp {
		app := newIdentityEditApp(t)
		seed = seedIdentityEdit(t, app)
		if c.prepare != nil {
			c.prepare(t, app, seed)
		}
		scenario.URL = "/api/collections/mail_identities/records/" + seed.annID.Id
		scenario.Body = strings.NewReader(c.body(t, app, seed))
		return app
	}
	scenario.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, _ *core.ServeEvent) {
		if c.actor != nil {
			scenario.Headers["Authorization"] = authTokenFor(t, c.actor(seed))
		}
	}
	scenario.AfterTestFunc = func(_ testing.TB, app *tests.TestApp, _ *http.Response) {
		if c.after != nil {
			c.after(t, app, seed)
		}
	}
	scenario.Test(t)
}

func asAnn(s *identityEditSeed) *core.Record   { return s.ann }
func asAdmin(s *identityEditSeed) *core.Record { return s.admin }

func bodyOf(json string) func(*testing.T, *tests.TestApp, *identityEditSeed) string {
	return func(*testing.T, *tests.TestApp, *identityEditSeed) string { return json }
}

func assertIdentityField(t *testing.T, app core.App, s *identityEditSeed, field, want string) {
	t.Helper()
	if got := mustRecord(t, app, "mail_identities", s.annID.Id).GetString(field); got != want {
		t.Fatalf("%s = %q, want %q", field, got, want)
	}
}

func manualAddressBody(t *testing.T, app *tests.TestApp, s *identityEditSeed) string {
	return `{"default_from":"` + s.manualID(t, app) + `"}`
}

func (s *identityEditSeed) manualID(t *testing.T, app core.App) string {
	t.Helper()
	return addressRowID(t, app, "annie", s.domain)
}

func addAnnManual(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
	t.Helper()
	saveRealRecord(t, app, "mail_addresses", map[string]any{
		"local_part": "annie", "domain": s.domain.Id, "mailbox": s.annID.GetString("inbox"), "source": addressSourceManual,
	})
}

func TestIdentityEdit_SelfSetsDefaultFrom(t *testing.T) {
	runIdentityEdit(t, identityEditCase{
		name: "self default_from", actor: asAnn, status: http.StatusOK, content: `"id"`,
		prepare: addAnnManual, body: manualAddressBody,
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			assertIdentityField(t, app, s, "default_from", s.manualID(t, app))
		},
	})
}

func TestIdentityEdit_DefaultFromMustDeliverToInbox(t *testing.T) {
	runIdentityEdit(t, identityEditCase{
		name: "foreign default_from", actor: asAnn, status: http.StatusBadRequest, content: `"default_from"`,
		body: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) string {
			return `{"default_from":"` + addressRowID(t, app, "team", s.domain) + `"}`
		},
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			assertIdentityField(t, app, s, "default_from", "")
		},
	})
}

func TestIdentityEdit_EmptyDefaultFromClears(t *testing.T) {
	runIdentityEdit(t, identityEditCase{
		name: "clear default_from", actor: asAnn, status: http.StatusOK, content: `"id"`,
		prepare: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			addAnnManual(t, app, s)
			s.annID.Set("default_from", s.manualID(t, app))
			if err := app.Save(s.annID); err != nil {
				t.Fatal(err)
			}
		},
		body: bodyOf(`{"default_from":""}`),
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			assertIdentityField(t, app, s, "default_from", "")
		},
	})
}

func TestIdentityEdit_MemberCannotRename(t *testing.T) {
	runIdentityEdit(t, identityEditCase{
		name: "member rename", actor: asAnn, status: http.StatusForbidden, content: `Only an admin`,
		body: bodyOf(`{"email_name":"anna"}`),
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			assertIdentityField(t, app, s, "email_name", "ann")
		},
	})
}

func TestIdentityEdit_ServerFieldsStayFixed(t *testing.T) {
	runIdentityEdit(t, identityEditCase{
		name: "server fields", actor: asAdmin, status: http.StatusOK, content: `"id"`,
		prepare: addAnnManual,
		body: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) string {
			return `{"user":"` + s.otherUser.Id + `","inbox":"` + s.shared.Id + `","retired":true,"default_from":"` + s.manualID(t, app) + `"}`
		},
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			assertIdentityField(t, app, s, "default_from", s.manualID(t, app))
			assertIdentityField(t, app, s, "user", s.ann.Id)
			assertIdentityField(t, app, s, "inbox", s.annInbox.Id)
			if mustRecord(t, app, "mail_identities", s.annID.Id).GetBool("retired") {
				t.Fatal("retired changed")
			}
		},
	})
}

func TestIdentityEdit_AdminRenameMovesRuleAddress(t *testing.T) {
	runIdentityEdit(t, identityEditCase{
		name: "admin rename", actor: asAdmin, status: http.StatusOK, content: `"anna"`,
		body: bodyOf(`{"email_name":"anna"}`),
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			row, _ := findAddressRow(app, "anna", s.domain.Id)
			if row == nil || row.GetString("source") != addressSourceRule {
				t.Fatalf("anna row = %v, want a rule row", row)
			}
			if old, _ := findAddressRow(app, "ann", s.domain.Id); old != nil && old.GetString("source") == addressSourceRule {
				t.Fatal("the old rule address still exists")
			}
		},
	})
}

func TestIdentityEdit_RenameRefusedByReservedConflict(t *testing.T) {
	runIdentityEdit(t, identityEditCase{
		name: "reserved conflict", actor: asAdmin, status: http.StatusBadRequest, content: `anna@example.org`,
		prepare: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			saveRealRecord(t, app, "mail_addresses", map[string]any{
				"local_part": "anna", "domain": s.domain.Id, "mailbox": s.shared.Id, "source": addressSourceManual,
			})
		},
		body: bodyOf(`{"email_name":"anna"}`),
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			assertIdentityField(t, app, s, "email_name", "ann")
		},
	})
}

func TestIdentityEdit_RenameRefusedWhenTaken(t *testing.T) {
	runIdentityEdit(t, identityEditCase{
		name: "taken", actor: asAdmin, status: http.StatusBadRequest, content: `"email_name"`,
		body: bodyOf(`{"email_name":"bob"}`),
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			assertIdentityField(t, app, s, "email_name", "ann")
		},
	})
}

func TestIdentityEdit_RenameNormalizesAndValidates(t *testing.T) {
	runIdentityEdit(t, identityEditCase{
		name: "normalizes", actor: asAdmin, status: http.StatusOK, content: `"anna"`,
		body: bodyOf(`{"email_name":" Anna "}`),
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			assertIdentityField(t, app, s, "email_name", "anna")
		},
	})
	runIdentityEdit(t, identityEditCase{
		name: "space inside", actor: asAdmin, status: http.StatusBadRequest, content: `"email_name"`,
		body: bodyOf(`{"email_name":"an na"}`),
	})
	runIdentityEdit(t, identityEditCase{
		name: "too long", actor: asAdmin, status: http.StatusBadRequest, content: `"email_name"`,
		body: bodyOf(`{"email_name":"` + strings.Repeat("a", 65) + `"}`),
	})
}

func TestIdentityEdit_AnonymousRefused(t *testing.T) {
	runIdentityEdit(t, identityEditCase{
		name: "anonymous", status: http.StatusNotFound, content: `"message"`,
		body: bodyOf(`{"email_name":"anna"}`),
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			assertIdentityField(t, app, s, "email_name", "ann")
		},
	})
}

// The edit hooks check every save of an identity, the server's own included.
// Offboarding writes a tombstone name that is not a person's choice.
func TestIdentityEdit_OffboardingStillRetires(t *testing.T) {
	app := newIdentityEditApp(t)
	s := seedIdentityEdit(t, app)
	if err := retireIdentity(app, s.ann.Id); err != nil {
		t.Fatalf("retire with the edit hooks on: %v", err)
	}
	identity := identityOf(t, app, s.ann.Id)
	if !identity.GetBool("retired") || identity.GetString("email_name") != "retired-"+identity.Id {
		t.Fatalf("identity = retired %v name %q", identity.GetBool("retired"), identity.GetString("email_name"))
	}
}

// repairEmailName derives its names, so the checks accept them.
func TestIdentityEdit_RepairedNameIsAccepted(t *testing.T) {
	app := newIdentityEditApp(t)
	s := seedIdentityEdit(t, app)
	identity := mustRecord(t, app, "mail_identities", s.annID.Id)
	if err := app.Delete(s.annInbox); err != nil {
		t.Fatal(err)
	}
	saveRealRecord(t, app, "mail_addresses", map[string]any{
		"local_part": "ann", "domain": s.domain.Id, "mailbox": s.shared.Id, "source": addressSourceManual,
	})
	identity = mustRecord(t, app, "mail_identities", identity.Id)
	identity.Set("inbox", "")
	if err := app.Save(identity); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureIdentityWith(app, s.ann, adoptOrCreateInbox); err != nil {
		t.Fatalf("repair with the edit hooks on: %v", err)
	}
	if got := identityOf(t, app, s.ann.Id); got.GetString("inbox") == "" || got.GetString("email_name") == "ann" {
		t.Fatalf("identity after repair = inbox %q name %q", got.GetString("inbox"), got.GetString("email_name"))
	}
}
