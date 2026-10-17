package mail

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func newGroupAddressTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := newRealSchemaApp(t)
	registerGroupAddressHooks(app)
	registerAddressHooks(app)
	registerIdentityHooks(app)
	registerRuleAddressHooks(app)
	return app
}

type groupAddressSeed struct {
	admin  *core.Record
	domain *core.Record
	ann    *core.Record
	inbox  *core.Record
	sales  *core.Record
}

// seedGroupAddress makes an everyone domain (the first domain), an admin, and
// ann, a member of the Sales group with her own inbox.
func seedGroupAddress(t *testing.T, app core.App) *groupAddressSeed {
	t.Helper()
	s := &groupAddressSeed{}
	s.domain = saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "q.example", "verified": true})
	s.admin = saveRealRecord(t, app, "users", map[string]any{
		"email": "admin@ext.example", "username": "adminuser", "name": "Admin", "role": "admin",
	})
	s.ann = seedGroupUser(t, app, "ann")
	identity := identityOf(t, app, s.ann.Id)
	if identity == nil || identity.GetString("inbox") == "" {
		t.Fatal("seed: ann has no inbox")
	}
	s.inbox = mustRecord(t, app, "mail_mailboxes", identity.GetString("inbox"))
	s.sales = seedGroup(t, app, "Sales", s.ann)
	return s
}

func saveGroupAddress(t *testing.T, app core.App, local string, domain, group *core.Record) *core.Record {
	t.Helper()
	return saveRealRecord(t, app, "mail_addresses", map[string]any{
		"local_part": local, "domain": domain.Id, "group": group.Id, "source": addressSourceManual,
	})
}

type groupAddressPost struct {
	name    string
	prepare func(t *testing.T, app *tests.TestApp, s *groupAddressSeed)
	body    func(s *groupAddressSeed) string
	status  int
	content string
	after   func(t *testing.T, app *tests.TestApp, s *groupAddressSeed)
}

func runGroupAddressPost(t *testing.T, c groupAddressPost) {
	t.Helper()
	var seed *groupAddressSeed
	scenario := tests.ApiScenario{
		Name:            c.name,
		Method:          http.MethodPost,
		URL:             "/api/collections/mail_addresses/records",
		ExpectedStatus:  c.status,
		ExpectedContent: []string{c.content},
		Headers:         map[string]string{"Content-Type": "application/json"},
	}
	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp {
		app := newGroupAddressTestApp(t)
		seed = seedGroupAddress(t, app)
		if c.prepare != nil {
			c.prepare(t, app, seed)
		}
		scenario.Body = strings.NewReader(c.body(seed))
		return app
	}
	scenario.BeforeTestFunc = func(_ testing.TB, _ *tests.TestApp, _ *core.ServeEvent) {
		scenario.Headers["Authorization"] = authTokenFor(t, seed.admin)
	}
	scenario.AfterTestFunc = func(_ testing.TB, app *tests.TestApp, _ *http.Response) {
		if c.after != nil {
			c.after(t, app, seed)
		}
	}
	scenario.Test(t)
}

func noAddressRow(local string) func(t *testing.T, app *tests.TestApp, s *groupAddressSeed) {
	return func(t *testing.T, app *tests.TestApp, s *groupAddressSeed) {
		t.Helper()
		row, err := findAddressRow(app, local, s.domain.Id)
		if err != nil {
			t.Fatal(err)
		}
		if row != nil && row.GetString("group") != "" {
			t.Fatalf("%s was saved as a group address: %v", local, row)
		}
	}
}

func TestGroupAddress_ExactlyOneTarget(t *testing.T) {
	t.Run("both", func(t *testing.T) {
		runGroupAddressPost(t, groupAddressPost{
			name: "mailbox and group", status: http.StatusBadRequest, content: `"group":{"code"`,
			body: func(s *groupAddressSeed) string {
				return `{"local_part":"sales","domain":"` + s.domain.Id + `","mailbox":"` + s.inbox.Id +
					`","group":"` + s.sales.Id + `"}`
			},
			after: func(t *testing.T, app *tests.TestApp, s *groupAddressSeed) {
				if row, _ := findAddressRow(app, "sales", s.domain.Id); row != nil {
					t.Fatalf("a row with two targets was saved: %v", row)
				}
			},
		})
	})
	t.Run("neither", func(t *testing.T) {
		runGroupAddressPost(t, groupAddressPost{
			name: "no target", status: http.StatusBadRequest, content: `"group":{"code"`,
			body: func(s *groupAddressSeed) string {
				return `{"local_part":"sales","domain":"` + s.domain.Id + `"}`
			},
			after: func(t *testing.T, app *tests.TestApp, s *groupAddressSeed) {
				if row, _ := findAddressRow(app, "sales", s.domain.Id); row != nil {
					t.Fatalf("a row with no target was saved: %v", row)
				}
			},
		})
	})
	t.Run("group only", func(t *testing.T) {
		runGroupAddressPost(t, groupAddressPost{
			name: "group target", status: http.StatusOK, content: `"local_part":"sales"`,
			body: func(s *groupAddressSeed) string {
				return `{"local_part":"sales","domain":"` + s.domain.Id + `","group":"` + s.sales.Id + `"}`
			},
			after: func(t *testing.T, app *tests.TestApp, s *groupAddressSeed) {
				row, err := findAddressRow(app, "sales", s.domain.Id)
				if err != nil || row == nil {
					t.Fatalf("group address missing: %v", err)
				}
				if row.GetString("group") != s.sales.Id || row.GetString("mailbox") != "" {
					t.Fatalf("group %q mailbox %q; want the Sales group and no inbox",
						row.GetString("group"), row.GetString("mailbox"))
				}
			},
		})
	})
}

func assertManualNotPrimary(t *testing.T, row *core.Record) {
	t.Helper()
	if row.GetString("source") != addressSourceManual || row.GetBool("is_primary") || row.GetString("via_group") != "" {
		t.Fatalf("source %q primary %v via_group %q; want manual, false, empty",
			row.GetString("source"), row.GetBool("is_primary"), row.GetString("via_group"))
	}
}

func TestGroupAddress_IsManualAndNotPrimary(t *testing.T) {
	t.Run("api", func(t *testing.T) {
		runGroupAddressPost(t, groupAddressPost{
			name: "rule-shaped group address", status: http.StatusOK, content: `"local_part":"sales"`,
			body: func(s *groupAddressSeed) string {
				return `{"local_part":"sales","domain":"` + s.domain.Id + `","group":"` + s.sales.Id +
					`","source":"rule","is_primary":true,"via_group":"` + s.sales.Id + `"}`
			},
			after: func(t *testing.T, app *tests.TestApp, s *groupAddressSeed) {
				row, err := findAddressRow(app, "sales", s.domain.Id)
				if err != nil || row == nil {
					t.Fatalf("group address missing: %v", err)
				}
				assertManualNotPrimary(t, row)
			},
		})
	})
	// The model hook, not only the request hook: a server save is held to it too.
	t.Run("server save", func(t *testing.T) {
		app := newGroupAddressTestApp(t)
		s := seedGroupAddress(t, app)
		saveRealRecord(t, app, "mail_addresses", map[string]any{
			"local_part": "sales", "domain": s.domain.Id, "group": s.sales.Id,
			"source": addressSourceRule, "is_primary": true, "via_group": s.sales.Id,
		})
		row, err := findAddressRow(app, "sales", s.domain.Id)
		if err != nil || row == nil {
			t.Fatalf("group address missing: %v", err)
		}
		assertManualNotPrimary(t, row)
	})
}

func TestGroupAddress_ReservedNameRefused(t *testing.T) {
	t.Run("everyone domain", func(t *testing.T) {
		runGroupAddressPost(t, groupAddressPost{
			name: "a person's email name", status: http.StatusBadRequest, content: `"local_part":{"code"`,
			body: func(s *groupAddressSeed) string {
				return `{"local_part":"ann","domain":"` + s.domain.Id + `","group":"` + s.sales.Id + `"}`
			},
			after: noAddressRow("ann"),
		})
	})
	// On a groups domain the name is reserved for a person who is not a
	// member, so no row holds it and only the reservation refuses it.
	t.Run("groups domain", func(t *testing.T) {
		var groupsDomain *core.Record
		runGroupAddressPost(t, groupAddressPost{
			name: "a non-member's email name", status: http.StatusBadRequest, content: `"address_reserved"`,
			prepare: func(t *testing.T, app *tests.TestApp, s *groupAddressSeed) {
				support := seedGroup(t, app, "Support")
				groupsDomain = saveGroupsDomain(t, app, "g.example", support)
			},
			body: func(s *groupAddressSeed) string {
				return `{"local_part":"ann","domain":"` + groupsDomain.Id + `","group":"` + s.sales.Id + `"}`
			},
			after: func(t *testing.T, app *tests.TestApp, s *groupAddressSeed) {
				if row, _ := findAddressRow(app, "ann", groupsDomain.Id); row != nil {
					t.Fatalf("ann@g.example was saved: %v", row)
				}
			},
		})
	})
}

func TestGroupAddress_TakenRefused(t *testing.T) {
	runGroupAddressPost(t, groupAddressPost{
		name: "an inbox address", status: http.StatusBadRequest, content: `"address_taken"`,
		prepare: func(t *testing.T, app *tests.TestApp, s *groupAddressSeed) {
			saveRealRecord(t, app, "mail_mailboxes", map[string]any{
				"address": "team", "domain": s.domain.Id, "type": "shared", "display_name": "Team", "name": "Team",
			})
		},
		body: func(s *groupAddressSeed) string {
			return `{"local_part":"team","domain":"` + s.domain.Id + `","group":"` + s.sales.Id + `"}`
		},
		after: noAddressRow("team"),
	})
}

func TestGroupAddress_NotASender(t *testing.T) {
	env, s := smtpSendEnv(t)
	sales := seedGroup(t, env.app, "Sales")
	group := saveGroupAddress(t, env.app, "sales", env.org, sales)

	t.Run("send endpoint", func(t *testing.T) {
		_, err := resolveSendAddress(env.app, env.inbox, group.Id, "")
		if !errors.Is(err, errAddressNotFound) {
			t.Fatalf("resolveSendAddress = %v, want errAddressNotFound", err)
		}
	})
	t.Run("resolver", func(t *testing.T) {
		if _, err := resolveAddress(env.app, "sales", "tinycld.org"); !errors.Is(err, errAddressNotFound) {
			t.Fatalf("resolveAddress = %v, want errAddressNotFound", err)
		}
		if _, err := loadResolvedAddress(env.app, group.Id); !errors.Is(err, errAddressNotFound) {
			t.Fatalf("loadResolvedAddress = %v, want errAddressNotFound", err)
		}
	})
	t.Run("smtp", func(t *testing.T) {
		var refusal *smtp.SMTPError
		err := s.Mail("sales@tinycld.org", nil)
		if !errors.As(err, &refusal) || refusal.Code != 550 || refusal.EnhancedCode != (smtp.EnhancedCode{5, 1, 0}) {
			t.Fatalf("MAIL FROM a group address = %v, want 550 5.1.0", err)
		}
	})
	t.Run("login", func(t *testing.T) {
		if got := loginIdentifier(env.app, "sales@tinycld.org"); got != "sales@tinycld.org" {
			t.Fatalf("loginIdentifier = %q, want it unchanged (unknown)", got)
		}
	})
}

func TestGroupAddress_GroupDeleteRemovesAddressKeepsMail(t *testing.T) {
	app := newMembershipTestApp(t)
	registerGroupAddressHooks(app)
	s := seedGroupAddress(t, app)
	group := saveGroupAddress(t, app, "sales", s.domain, s.sales)

	msg := &InboundMessage{
		From:      Recipient{Email: "sender@sender.example"},
		To:        []Recipient{{Email: "sales@q.example"}},
		Subject:   "To the group",
		TextBody:  "body",
		MessageID: "<group1@sender.example>",
		Date:      time.Now().UTC().Format(time.RFC3339),
	}
	if err := processInboundForMailbox(app, s.inbox, msg, group.Id); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	message, err := app.FindFirstRecordByFilter("mail_messages", "message_id = {:id}", map[string]any{"id": msg.MessageID})
	if err != nil {
		t.Fatal(err)
	}
	if message.GetString("delivered_to") != group.Id {
		t.Fatalf("precondition: delivered_to = %q, want the group address", message.GetString("delivered_to"))
	}

	if err := app.Delete(reloaded(t, app, s.sales)); err != nil {
		t.Fatalf("deleting the group: %v", err)
	}
	if _, err := app.FindRecordById("mail_addresses", group.Id); err == nil {
		t.Fatal("the group address outlived its group")
	}
	kept, err := app.FindRecordById("mail_messages", message.Id)
	if err != nil {
		t.Fatalf("the message was deleted with the group: %v", err)
	}
	if got := kept.GetString("delivered_to"); got != "" {
		t.Fatalf("delivered_to = %q, want empty", got)
	}
}

// An identity with no inbox (a retired one) matches a group address's empty
// mailbox, so the checks must refuse a group address outright.
func TestIdentityEdit_DefaultFromNotAGroupAddress(t *testing.T) {
	t.Run("no inbox", func(t *testing.T) {
		app := newGroupAddressTestApp(t)
		s := seedGroupAddress(t, app)
		group := saveGroupAddress(t, app, "sales", s.domain, s.sales)
		identity := core.NewRecord(mustCollection(t, app, "mail_identities"))
		identity.Set("user", s.ann.Id)
		identity.Set("inbox", "")
		identity.Set("retired", true)

		identity.Set("default_from", group.Id)
		if err := checkDefaultFrom(app, identity); err == nil {
			t.Fatal("checkDefaultFrom accepted a group address")
		}
		identity.Set("default_from", "")
		identity.Set("hidden_address_views", []string{group.Id})
		if err := checkHiddenAddressViews(app, identity); err == nil {
			t.Fatal("checkHiddenAddressViews accepted a group address")
		}
	})

	groupAddressID := func(t *testing.T, app *tests.TestApp, s *identityEditSeed) string {
		return addressRowID(t, app, "sales", s.domain)
	}
	addGroupAddress := func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
		saveGroupAddress(t, app, "sales", s.domain, seedGroup(t, app, "Sales", s.ann))
	}
	t.Run("live default_from", func(t *testing.T) {
		runIdentityEdit(t, identityEditCase{
			name: "group default_from", actor: asAnn, status: http.StatusBadRequest, content: `"default_from"`,
			prepare: addGroupAddress,
			body: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) string {
				return `{"default_from":"` + groupAddressID(t, app, s) + `"}`
			},
			after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
				assertIdentityField(t, app, s, "default_from", "")
			},
		})
	})
	t.Run("live hidden_address_views", func(t *testing.T) {
		runIdentityEdit(t, identityEditCase{
			name: "group hidden view", actor: asAnn, status: http.StatusBadRequest, content: `"hidden_address_views"`,
			prepare: addGroupAddress,
			body: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) string {
				return `{"hidden_address_views":["` + groupAddressID(t, app, s) + `"]}`
			},
			after: func(t *testing.T, app *tests.TestApp, s *identityEditSeed) {
				hidden := mustRecord(t, app, "mail_identities", s.annID.Id).GetStringSlice("hidden_address_views")
				if len(hidden) != 0 {
					t.Fatalf("hidden_address_views = %v, want none", hidden)
				}
			},
		})
	})
}
