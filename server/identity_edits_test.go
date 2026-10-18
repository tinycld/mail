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

func mustRecord(t testing.TB, app core.App, collection, id string) *core.Record {
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
	var second *core.Record
	runIdentityEdit(t, identityEditCase{
		name: "admin rename", actor: asAdmin, status: http.StatusOK, content: `"anna"`,
		prepare: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			second = saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "second.org", "verified": true, "coverage": coverageEveryone})
			if row, err := findAddressRow(app, "ann", second.Id); err != nil || row == nil {
				t.Fatalf("seed: ann@second.org = %v %v", row, err)
			}
		},
		body: bodyOf(`{"email_name":"anna"}`),
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			row, err := findAddressRow(app, "anna", second.Id)
			if err != nil || row == nil || row.GetString("source") != addressSourceRule {
				t.Fatalf("anna@second.org = %v %v, want a rule row", row, err)
			}
			old, err := findAddressRow(app, "ann", second.Id)
			if err != nil || old != nil {
				t.Fatalf("ann@second.org = %v %v, want no row", old, err)
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

func TestIdentityEdit_RetiredIdentityCannotChange(t *testing.T) {
	var tombstone string
	runIdentityEdit(t, identityEditCase{
		name: "retired rename", actor: asAdmin, status: http.StatusForbidden, content: `This person has left`,
		prepare: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			if err := retireIdentity(app, s.ann.Id); err != nil {
				t.Fatal(err)
			}
			tombstone = "retired-" + s.annID.Id
		},
		body: bodyOf(`{"email_name":"carol"}`),
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			assertIdentityField(t, app, s, "email_name", tombstone)
		},
	})
}

func primaryRowOf(t *testing.T, app core.App, inboxID string) *core.Record {
	t.Helper()
	row, err := app.FindFirstRecordByFilter("mail_addresses",
		"mailbox = {:inbox} && is_primary = true", map[string]any{"inbox": inboxID})
	if err != nil {
		t.Fatalf("primary row of %s: %v", inboxID, err)
	}
	return row
}

func setDefaultFrom(t *testing.T, app core.App, s *identityEditSeed, addressID string) {
	t.Helper()
	identity := mustRecord(t, app, "mail_identities", s.annID.Id)
	identity.Set("default_from", addressID)
	if err := app.Save(identity); err != nil {
		t.Fatal(err)
	}
}

// The main address follows the email name: the inbox's primary row moves in
// place, so a Default From that pointed at it now sends as the new name.
func TestIdentityEdit_RenameMovesMainAddress(t *testing.T) {
	var primaryID string
	runIdentityEdit(t, identityEditCase{
		name: "rename moves main address", actor: asAdmin, status: http.StatusOK, content: `"anna"`,
		prepare: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			primaryID = primaryRowOf(t, app, s.annInbox.Id).Id
			setDefaultFrom(t, app, s, primaryID)
		},
		body: bodyOf(`{"email_name":"anna"}`),
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			if got := mustRecord(t, app, "mail_mailboxes", s.annInbox.Id).GetString("address"); got != "anna" {
				t.Fatalf("inbox address = %q, want anna", got)
			}
			primary := primaryRowOf(t, app, s.annInbox.Id)
			if primary.Id != primaryID || primary.GetString("local_part") != "anna" {
				t.Fatalf("primary = %s %q, want %s anna", primary.Id, primary.GetString("local_part"), primaryID)
			}
			if old, _ := findAddressRow(app, "ann", s.domain.Id); old != nil {
				t.Fatalf("ann@example.org still exists: %s %s", old.GetString("source"), old.Id)
			}
			assertIdentityField(t, app, s, "default_from", primaryID)
			from, err := defaultSendAddress(app, mustRecord(t, app, "mail_mailboxes", s.annInbox.Id))
			if err != nil || from.Email() != "anna@example.org" {
				t.Fatalf("default send address = %v %v, want anna@example.org", from, err)
			}
		},
	})
}

// Renaming back to a name the inbox kept as an added address takes that row
// over: the kept row goes, and the main address moves onto the name.
func TestIdentityEdit_RenameBackTakesOverKeptAddress(t *testing.T) {
	var primaryID string
	runIdentityEdit(t, identityEditCase{
		name: "rename back", actor: asAdmin, status: http.StatusOK, content: `"ann"`,
		prepare: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			primaryID = primaryRowOf(t, app, s.annInbox.Id).Id
			identity := mustRecord(t, app, "mail_identities", s.annID.Id)
			identity.Set("email_name", "anna")
			if err := app.Save(identity); err != nil {
				t.Fatalf("first rename: %v", err)
			}
			kept := saveRealRecord(t, app, "mail_addresses", map[string]any{
				"local_part": "ann", "domain": s.domain.Id, "mailbox": s.annInbox.Id, "source": addressSourceManual,
			})
			setDefaultFrom(t, app, s, kept.Id)
		},
		body: bodyOf(`{"email_name":"ann"}`),
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			primary := primaryRowOf(t, app, s.annInbox.Id)
			if primary.Id != primaryID || primary.GetString("local_part") != "ann" {
				t.Fatalf("primary = %s %q, want %s ann", primary.Id, primary.GetString("local_part"), primaryID)
			}
			rows, err := app.FindRecordsByFilter("mail_addresses", "mailbox = {:inbox}", "", 0, 0,
				map[string]any{"inbox": s.annInbox.Id})
			if err != nil || len(rows) != 1 {
				t.Fatalf("inbox rows = %d %v, want only the main address", len(rows), err)
			}
			assertIdentityField(t, app, s, "default_from", primaryID)
		},
	})
}

// Another inbox's address on the main address's domain blocks the move even
// when that domain gives no addresses by rule.
func TestIdentityEdit_RenameRefusedByMainDomainHolder(t *testing.T) {
	runIdentityEdit(t, identityEditCase{
		name: "main domain holder", actor: asAdmin, status: http.StatusBadRequest, content: `anna@example.org`,
		prepare: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			domain := reloaded(t, app, s.domain)
			domain.Set("coverage", coverageManual)
			if err := app.Save(domain); err != nil {
				t.Fatal(err)
			}
			saveRealRecord(t, app, "mail_addresses", map[string]any{
				"local_part": "anna", "domain": s.domain.Id, "mailbox": s.shared.Id, "source": addressSourceManual,
			})
		},
		body: bodyOf(`{"email_name":"anna"}`),
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			assertIdentityField(t, app, s, "email_name", "ann")
			if got := mustRecord(t, app, "mail_mailboxes", s.annInbox.Id).GetString("address"); got != "ann" {
				t.Fatalf("inbox address = %q, want ann", got)
			}
		},
	})
}

// Another live identity has the name but no address on a domain with a rule
// or on the main address's domain: only the identity check refuses it.
func TestIdentityEdit_RenameRefusedByAnotherIdentityName(t *testing.T) {
	runIdentityEdit(t, identityEditCase{
		name: "identity name taken", actor: asAdmin, status: http.StatusBadRequest, content: `email_name_taken`,
		prepare: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			other := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "other.org", "verified": true, "coverage": coverageManual})
			bobInbox := reloaded(t, app, mustRecord(t, app, "mail_mailboxes", identityOf(t, app, s.otherUser.Id).GetString("inbox")))
			bobInbox.Set("domain", other.Id)
			if err := app.Save(bobInbox); err != nil {
				t.Fatal(err)
			}
			// The rule row the move left behind; deleting a rule row does not
			// reconcile, so Bob keeps no address on example.org.
			if row, _ := findAddressRow(app, "bob", s.domain.Id); row != nil {
				if err := app.Delete(row); err != nil {
					t.Fatal(err)
				}
			}
		},
		body: bodyOf(`{"email_name":"bob"}`),
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			assertIdentityField(t, app, s, "email_name", "ann")
		},
	})
}

func TestIdentityEdit_SelfSetsAddressViews(t *testing.T) {
	runIdentityEdit(t, identityEditCase{
		name: "self address views", actor: asAnn, status: http.StatusOK, content: `"address_views":true`,
		prepare: addAnnManual,
		body: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) string {
			return `{"address_views":true,"hidden_address_views":["` + s.manualID(t, app) + `"]}`
		},
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			identity := mustRecord(t, app, "mail_identities", s.annID.Id)
			if !identity.GetBool("address_views") {
				t.Fatal("address_views = false, want true")
			}
			hidden := identity.GetStringSlice("hidden_address_views")
			if len(hidden) != 1 || hidden[0] != s.manualID(t, app) {
				t.Fatalf("hidden_address_views = %v, want [%s]", hidden, s.manualID(t, app))
			}
		},
	})
}

func TestIdentityEdit_HiddenMustBeOwnAddress(t *testing.T) {
	runIdentityEdit(t, identityEditCase{
		name: "foreign hidden address", actor: asAnn, status: http.StatusBadRequest, content: `"hidden_address_views"`,
		body: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) string {
			return `{"hidden_address_views":["` + addressRowID(t, app, "team", s.domain) + `"]}`
		},
		after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
			hidden := mustRecord(t, app, "mail_identities", s.annID.Id).GetStringSlice("hidden_address_views")
			if len(hidden) != 0 {
				t.Fatalf("hidden_address_views = %v, want none", hidden)
			}
		},
	})
}
