package mail

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func newAddressTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := newRealSchemaApp(t)
	registerAddressHooks(app)
	return app
}

func TestResolveAddress_FindsPrimaryAndAddedAddresses(t *testing.T) {
	t.Parallel()
	app := newAddressTestApp(t)
	org := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.org", "verified": true})
	com := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "tinycld.com", "verified": true})
	inbox := saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "nathan", "domain": org.Id, "type": "personal", "display_name": "Nathan", "name": "Nathan",
	})
	saveRealRecord(t, app, "mail_addresses", map[string]any{
		"local_part": "nathan", "domain": com.Id, "mailbox": inbox.Id, "source": addressSourceManual,
	})

	for _, tc := range []struct{ local, domain string }{
		{"nathan", "tinycld.org"},
		{"Nathan", "TINYCLD.COM"},
	} {
		got, err := resolveAddress(app, tc.local, tc.domain)
		if err != nil {
			t.Fatalf("%s@%s: %v", tc.local, tc.domain, err)
		}
		if got.Mailbox.Id != inbox.Id {
			t.Fatalf("%s@%s resolved to mailbox %s, want %s", tc.local, tc.domain, got.Mailbox.Id, inbox.Id)
		}
		if want := strings.ToLower(tc.local + "@" + tc.domain); got.Email() != want {
			t.Fatalf("Email() = %q, want %q", got.Email(), want)
		}
	}
	if _, err := resolveAddress(app, "nobody", "tinycld.org"); err == nil {
		t.Fatal("an unknown address must not resolve")
	}
}

func TestMailboxCreate_SavesPrimaryAddress(t *testing.T) {
	t.Parallel()
	app := newAddressTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org"})
	shared := saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "Support", "domain": domain.Id, "type": "shared", "display_name": "Support", "name": "Support",
	})

	row, err := findAddressRow(app, "support", domain.Id)
	if err != nil || row == nil {
		t.Fatalf("no primary address row for a new mailbox: %v", err)
	}
	if row.GetString("mailbox") != shared.Id || !row.GetBool("is_primary") || row.GetString("source") != addressSourceManual {
		t.Fatalf("primary row = mailbox %s primary %v source %q", row.GetString("mailbox"), row.GetBool("is_primary"), row.GetString("source"))
	}
}

func TestMailboxUpdate_MovesPrimaryAddress(t *testing.T) {
	t.Parallel()
	app := newAddressTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org"})
	shared := saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "support", "domain": domain.Id, "type": "shared", "display_name": "Support", "name": "Support",
	})
	shared.Set("address", "help")
	if err := app.Save(shared); err != nil {
		t.Fatal(err)
	}
	if row, _ := findAddressRow(app, "support", domain.Id); row != nil {
		t.Fatal("the old address still routes after the mailbox address changed")
	}
	if row, _ := findAddressRow(app, "help", domain.Id); row == nil || row.GetString("mailbox") != shared.Id {
		t.Fatal("the new mailbox address does not route to the mailbox")
	}
}

func TestAddressCreate_RefusesTakenAddress(t *testing.T) {
	t.Parallel()
	app := newAddressTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org"})
	a := saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "a", "domain": domain.Id, "type": "shared", "display_name": "A", "name": "A"})
	b := saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "b", "domain": domain.Id, "type": "shared", "display_name": "B", "name": "B"})
	saveRealRecord(t, app, "mail_addresses", map[string]any{"local_part": "help", "domain": domain.Id, "mailbox": a.Id, "source": addressSourceManual})

	col, _ := app.FindCollectionByNameOrId("mail_addresses")
	dup := core.NewRecord(col)
	dup.Set("local_part", "HELP")
	dup.Set("domain", domain.Id)
	dup.Set("mailbox", b.Id)
	dup.Set("source", addressSourceManual)
	if err := app.Save(dup); err == nil {
		t.Fatal("a second inbox took an address that already exists")
	}

	mb := core.NewRecord(mustCollection(t, app, "mail_mailboxes"))
	mb.Set("address", "help")
	mb.Set("domain", domain.Id)
	mb.Set("type", "shared")
	mb.Set("display_name", "Help")
	mb.Set("name", "Help")
	if err := app.Save(mb); err == nil {
		t.Fatal("a new mailbox took an address that is already an added address")
	}
}

// On a domain that gives everyone an address, a person's email name is
// reserved for that person, even before the rule row exists.
func TestAddressCreate_RefusesReservedEmailName(t *testing.T) {
	t.Parallel()
	app := newAddressTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org", "coverage": coverageEveryone, "verified": true})
	user := saveRealRecord(t, app, "users", map[string]any{"email": "holly@example.net", "username": "holly", "name": "Holly", "role": "member"})
	inbox := saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "holly", "domain": domain.Id, "type": "personal", "display_name": "Holly", "name": "Holly"})
	saveRealRecord(t, app, "mail_identities", map[string]any{"user": user.Id, "inbox": inbox.Id, "email_name": "holly"})
	other := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.com", "coverage": coverageEveryone, "verified": true})

	err := checkAddressAvailable(app, "holly", other.Id, "", "")
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("want a reserved-name refusal, got %v", err)
	}
	if err := checkAddressAvailable(app, "holly", other.Id, inbox.Id, ""); err != nil {
		t.Fatalf("the person's own inbox must be allowed its email name: %v", err)
	}
	manual := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.net", "coverage": coverageManual})
	if err := checkAddressAvailable(app, "holly", manual.Id, "", ""); err != nil {
		t.Fatalf("a manual domain reserves nothing: %v", err)
	}
}

func TestLegacyAlias_IsMirroredIntoAddresses(t *testing.T) {
	t.Parallel()
	app := newAddressTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org"})
	mb := saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "nathan", "domain": domain.Id, "type": "personal", "display_name": "N", "name": "N"})

	alias := saveRealRecord(t, app, "mail_mailbox_aliases", map[string]any{"mailbox": mb.Id, "address": "Admin"})
	if row, _ := findAddressRow(app, "admin", domain.Id); row == nil || row.GetString("mailbox") != mb.Id {
		t.Fatal("an alias written by a released client does not route")
	}
	if err := app.Delete(alias); err != nil {
		t.Fatal(err)
	}
	if row, _ := findAddressRow(app, "admin", domain.Id); row != nil {
		t.Fatal("a deleted legacy alias still routes")
	}
}

func TestDomainCreate_Defaults(t *testing.T) {
	t.Parallel()
	app := newAddressTestApp(t)
	first := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "first.example"})
	second := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "second.example"})
	if first.GetString("coverage") != coverageEveryone || first.GetInt("rank") != 1 {
		t.Fatalf("first domain = %q rank %d; want everyone rank 1", first.GetString("coverage"), first.GetInt("rank"))
	}
	if second.GetString("coverage") != coverageManual || second.GetInt("rank") != 2 {
		t.Fatalf("second domain = %q rank %d; want manual rank 2", second.GetString("coverage"), second.GetInt("rank"))
	}
}

func mustCollection(t *testing.T, app core.App, name string) *core.Collection {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		t.Fatal(err)
	}
	return col
}

type addressAPISeed struct {
	admin   *core.Record
	domain  *core.Record
	mailbox *core.Record
	rule    *core.Record
	primary *core.Record
	manual  *core.Record
}

func seedAddressAPI(t *testing.T, app *tests.TestApp) *addressAPISeed {
	t.Helper()
	s := &addressAPISeed{}
	s.admin = saveRealRecord(t, app, "users", map[string]any{"email": "admin@example.net", "username": "adminuser", "name": "Admin", "role": "admin"})
	s.domain = saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org"})
	s.mailbox = saveRealRecord(t, app, "mail_mailboxes", map[string]any{
		"address": "team", "domain": s.domain.Id, "type": "shared", "display_name": "Team", "name": "Team",
	})
	s.primary, _ = findAddressRow(app, "team", s.domain.Id)
	if s.primary == nil {
		t.Fatal("seed: no primary address row")
	}
	s.rule = saveRealRecord(t, app, "mail_addresses", map[string]any{
		"local_part": "ruled", "domain": s.domain.Id, "mailbox": s.mailbox.Id, "source": addressSourceRule,
	})
	s.manual = saveRealRecord(t, app, "mail_addresses", map[string]any{
		"local_part": "extra", "domain": s.domain.Id, "mailbox": s.mailbox.Id, "source": addressSourceManual,
	})
	return s
}

func runAddressAPIScenario(
	t *testing.T,
	method string,
	body string,
	status int,
	content string,
	target func(s *addressAPISeed) string,
	after func(t *testing.T, app *tests.TestApp, s *addressAPISeed),
) {
	t.Helper()
	var seed *addressAPISeed
	scenario := tests.ApiScenario{
		Name:            method + " mail_addresses",
		Method:          method,
		Body:            strings.NewReader(body),
		ExpectedStatus:  status,
		ExpectedContent: []string{content},
		Headers:         map[string]string{"Content-Type": "application/json"},
	}
	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp {
		app := newAddressTestApp(t)
		seed = seedAddressAPI(t, app)
		scenario.URL = "/api/collections/mail_addresses/records" + target(seed)
		return app
	}
	scenario.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, _ *core.ServeEvent) {
		scenario.Headers["Authorization"] = authTokenFor(t, seed.admin)
	}
	scenario.AfterTestFunc = func(_ testing.TB, app *tests.TestApp, _ *http.Response) {
		if after != nil {
			after(t, app, seed)
		}
	}
	scenario.Test(t)
}

func TestAddressAPI_AdminCannotDeleteRuleRow(t *testing.T) {
	t.Parallel()
	runAddressAPIScenario(t, http.MethodDelete, "", http.StatusForbidden, `"message"`,
		func(s *addressAPISeed) string { return "/" + s.rule.Id },
		func(t *testing.T, app *tests.TestApp, s *addressAPISeed) {
			if _, err := app.FindRecordById("mail_addresses", s.rule.Id); err != nil {
				t.Fatalf("the rule row was deleted: %v", err)
			}
		})
}

func TestAddressAPI_AdminCannotDeletePrimaryRow(t *testing.T) {
	t.Parallel()
	runAddressAPIScenario(t, http.MethodDelete, "", http.StatusForbidden, `"message"`,
		func(s *addressAPISeed) string { return "/" + s.primary.Id },
		func(t *testing.T, app *tests.TestApp, s *addressAPISeed) {
			if _, err := app.FindRecordById("mail_addresses", s.primary.Id); err != nil {
				t.Fatalf("the primary row was deleted: %v", err)
			}
		})
}

func TestAddressAPI_AdminCanDeleteManualRow(t *testing.T) {
	t.Parallel()
	runAddressAPIScenario(t, http.MethodDelete, "", http.StatusNoContent, "",
		func(s *addressAPISeed) string { return "/" + s.manual.Id },
		func(t *testing.T, app *tests.TestApp, s *addressAPISeed) {
			if _, err := app.FindRecordById("mail_addresses", s.manual.Id); err == nil {
				t.Fatal("the manual row was not deleted")
			}
		})
}

func TestAddressAPI_CreateForcesManualNonPrimary(t *testing.T) {
	t.Parallel()
	var seed *addressAPISeed
	scenario := tests.ApiScenario{
		Name:            "POST mail_addresses",
		Method:          http.MethodPost,
		URL:             "/api/collections/mail_addresses/records",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"local_part":"sneaky"`},
		Headers:         map[string]string{"Content-Type": "application/json"},
	}
	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp {
		app := newAddressTestApp(t)
		seed = seedAddressAPI(t, app)
		scenario.Body = strings.NewReader(`{"local_part":"sneaky","domain":"` + seed.domain.Id +
			`","mailbox":"` + seed.mailbox.Id + `","source":"rule","is_primary":true}`)
		return app
	}
	scenario.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, _ *core.ServeEvent) {
		scenario.Headers["Authorization"] = authTokenFor(t, seed.admin)
	}
	scenario.AfterTestFunc = func(_ testing.TB, app *tests.TestApp, _ *http.Response) {
		row, err := findAddressRow(app, "sneaky", seed.domain.Id)
		if err != nil || row == nil {
			t.Fatalf("created row missing: %v", err)
		}
		if row.GetString("source") != addressSourceManual || row.GetBool("is_primary") {
			t.Fatalf("source %q primary %v; want manual, false", row.GetString("source"), row.GetBool("is_primary"))
		}
	}
	scenario.Test(t)
}

func TestMailboxCreate_RefusesUnroutableAddress(t *testing.T) {
	t.Parallel()
	app := newAddressTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org"})
	mb := core.NewRecord(mustCollection(t, app, "mail_mailboxes"))
	mb.Set("address", "a+b")
	mb.Set("domain", domain.Id)
	mb.Set("type", "shared")
	mb.Set("display_name", "AB")
	mb.Set("name", "AB")
	err := app.Save(mb)
	if err == nil || !strings.Contains(err.Error(), "not a valid mailbox address") {
		t.Fatalf("want an invalid-address refusal, got %v", err)
	}
}

func TestMailboxUpdate_LegacyInvalidAddressStillAllowsOtherEdits(t *testing.T) {
	t.Parallel()
	app := newAddressTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org"})
	mb := core.NewRecord(mustCollection(t, app, "mail_mailboxes"))
	mb.Set("address", "sales")
	mb.Set("domain", domain.Id)
	mb.Set("type", "shared")
	mb.Set("display_name", "Sales")
	mb.Set("name", "Sales")
	if err := app.Save(mb); err != nil {
		t.Fatal(err)
	}
	// Raw SQL stands in for a row saved before the address rule existed.
	if _, err := app.DB().NewQuery("UPDATE mail_mailboxes SET address = 'sales+eu' WHERE id = {:id}").
		Bind(dbx.Params{"id": mb.Id}).Execute(); err != nil {
		t.Fatal(err)
	}
	mb, err := app.FindRecordById("mail_mailboxes", mb.Id)
	if err != nil {
		t.Fatal(err)
	}
	mb.Set("display_name", "Sales EU")
	if err := app.Save(mb); err != nil {
		t.Fatalf("a display-name edit must not trip the address rule: %v", err)
	}
}

func TestMailboxUpdate_RenameToOwnAddedAddressSaysRemoveFirst(t *testing.T) {
	t.Parallel()
	app := newAddressTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org"})
	mb := saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "main", "domain": domain.Id, "type": "shared", "display_name": "M", "name": "M"})
	saveRealRecord(t, app, "mail_addresses", map[string]any{"local_part": "extra", "domain": domain.Id, "mailbox": mb.Id, "source": addressSourceManual})
	mb.Set("address", "extra")
	err := app.Save(mb)
	if err == nil || !strings.Contains(err.Error(), "remove that added address first") {
		t.Fatalf("got %v", err)
	}
}

func TestLegacyAlias_RefusedWhenAddressTaken(t *testing.T) {
	t.Parallel()
	app := newAddressTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "example.org"})
	a := saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "a", "domain": domain.Id, "type": "shared", "display_name": "A", "name": "A"})
	b := saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "b", "domain": domain.Id, "type": "shared", "display_name": "B", "name": "B"})
	saveRealRecord(t, app, "mail_addresses", map[string]any{"local_part": "help", "domain": domain.Id, "mailbox": a.Id, "source": addressSourceManual})

	col := mustCollection(t, app, "mail_mailbox_aliases")
	alias := core.NewRecord(col)
	alias.Set("mailbox", b.Id)
	alias.Set("address", "help")
	if err := app.Save(alias); err == nil {
		t.Fatal("a legacy alias took an address that already exists")
	}
}

func TestLegacyAlias_DeleteAfterMailboxDomainChange(t *testing.T) {
	t.Parallel()
	app := newAddressTestApp(t)
	d1 := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "one.example"})
	d2 := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "two.example"})
	mb := saveRealRecord(t, app, "mail_mailboxes", map[string]any{"address": "n", "domain": d1.Id, "type": "personal", "display_name": "N", "name": "N"})
	alias := saveRealRecord(t, app, "mail_mailbox_aliases", map[string]any{"mailbox": mb.Id, "address": "admin"})
	mb.Set("domain", d2.Id)
	if err := app.Save(mb); err != nil {
		t.Fatal(err)
	}
	if err := app.Delete(alias); err != nil {
		t.Fatal(err)
	}
	if row, _ := findAddressRow(app, "admin", d1.Id); row != nil {
		t.Fatal("orphan address row left behind")
	}
}
