package mail

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// padID normalises a test fixture ID to PocketBase's default format:
// exactly 15 lowercase alphanumeric characters.
func padID(id string) string {
	lower := strings.ToLower(id)
	stripped := make([]byte, 0, len(lower))
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			stripped = append(stripped, c)
		}
	}
	out := string(stripped)
	if len(out) >= 15 {
		return out[:15]
	}
	return out + strings.Repeat("x", 15-len(out))
}

func setupAliasTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("failed to create test app: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	// mail_domains — mirrors the shipped migration's fields these tests touch;
	// there is no org field (single-org: the deployment IS the org).
	domains := core.NewBaseCollection("mail_domains")
	domains.Fields.Add(&core.TextField{Name: "domain", Required: true})
	if err := app.Save(domains); err != nil {
		t.Fatalf("failed to save mail_domains: %v", err)
	}

	// mail_mailboxes
	mailboxes := core.NewBaseCollection("mail_mailboxes")
	mailboxes.Fields.Add(&core.TextField{Name: "address", Required: true})
	mailboxes.Fields.Add(&core.RelationField{
		Name:          "domain",
		Required:      true,
		CollectionId:  domains.Id,
		CascadeDelete: true,
		MaxSelect:     1,
	})
	mailboxes.Fields.Add(&core.TextField{Name: "display_name"})
	mailboxes.Fields.Add(&core.TextField{Name: "type"})
	if err := app.Save(mailboxes); err != nil {
		t.Fatalf("failed to save mail_mailboxes: %v", err)
	}

	// mail_mailbox_aliases
	aliases := core.NewBaseCollection("mail_mailbox_aliases")
	aliases.Fields.Add(&core.RelationField{
		Name:          "mailbox",
		Required:      true,
		CollectionId:  mailboxes.Id,
		CascadeDelete: true,
		MaxSelect:     1,
	})
	aliases.Fields.Add(&core.TextField{Name: "address", Required: true})
	if err := app.Save(aliases); err != nil {
		t.Fatalf("failed to save mail_mailbox_aliases: %v", err)
	}

	addAddressFixture(t, app)

	return app
}

// --- helpers ---

func seedDomainAndMailbox(t *testing.T, app core.App, domainStr, localPart, mailboxID string) string {
	t.Helper()

	domainsCol, err := app.FindCollectionByNameOrId("mail_domains")
	if err != nil {
		t.Fatalf("mail_domains collection missing: %v", err)
	}
	domain := core.NewRecord(domainsCol)
	domain.Set("domain", domainStr)
	if err := app.Save(domain); err != nil {
		t.Fatalf("failed to save domain %s: %v", domainStr, err)
	}

	mailboxesCol, err := app.FindCollectionByNameOrId("mail_mailboxes")
	if err != nil {
		t.Fatalf("mail_mailboxes collection missing: %v", err)
	}
	mailbox := core.NewRecord(mailboxesCol)
	mailbox.Id = padID(mailboxID)
	mailbox.Set("address", localPart)
	mailbox.Set("domain", domain.Id)
	mailbox.Set("type", "personal")
	if err := app.Save(mailbox); err != nil {
		t.Fatalf("failed to save mailbox %s@%s: %v", localPart, domainStr, err)
	}

	return domain.Id
}

func seedAlias(t *testing.T, app core.App, mailboxID, address, aliasID string) {
	t.Helper()

	aliasesCol, err := app.FindCollectionByNameOrId("mail_mailbox_aliases")
	if err != nil {
		t.Fatalf("mail_mailbox_aliases collection missing: %v", err)
	}
	alias := core.NewRecord(aliasesCol)
	alias.Id = padID(aliasID)
	alias.Set("mailbox", padID(mailboxID))
	alias.Set("address", address)
	if err := app.Save(alias); err != nil {
		t.Fatalf("failed to save alias %s: %v", address, err)
	}
}
