package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// addAddressFixture adds the address tables to a hand-built fixture and
// registers the hooks that keep them in step with mailboxes, so tests built on
// setupAliasTestApp resolve a mailbox's address through mail_addresses the
// same way production does.
func addAddressFixture(t *testing.T, app core.App) {
	t.Helper()
	domains, err := app.FindCollectionByNameOrId("mail_domains")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []core.Field{
		&core.SelectField{Name: "coverage", MaxSelect: 1, Values: []string{coverageEveryone, coverageManual}},
		&core.NumberField{Name: "rank", OnlyInt: true},
		&core.BoolField{Name: "verified"},
	} {
		if domains.Fields.GetByName(field.GetName()) == nil {
			domains.Fields.Add(field)
		}
	}
	if err := app.Save(domains); err != nil {
		t.Fatal(err)
	}
	mailboxes, err := app.FindCollectionByNameOrId("mail_mailboxes")
	if err != nil {
		t.Fatal(err)
	}

	addresses := core.NewBaseCollection("mail_addresses")
	addresses.Fields.Add(&core.TextField{Name: "local_part", Required: true})
	addresses.Fields.Add(&core.RelationField{Name: "domain", Required: true, CollectionId: domains.Id, CascadeDelete: true, MaxSelect: 1})
	addresses.Fields.Add(&core.RelationField{Name: "mailbox", Required: true, CollectionId: mailboxes.Id, CascadeDelete: true, MaxSelect: 1})
	addresses.Fields.Add(&core.SelectField{Name: "source", MaxSelect: 1, Values: []string{addressSourceManual, addressSourceRule}})
	addresses.Fields.Add(&core.BoolField{Name: "is_primary"})
	addresses.AddIndex("idx_fixture_addresses_local_domain", true, "`local_part`, `domain`", "")
	if err := app.Save(addresses); err != nil {
		t.Fatal(err)
	}

	identities := core.NewBaseCollection("mail_identities")
	identities.Fields.Add(&core.TextField{Name: "user", Required: true})
	identities.Fields.Add(&core.RelationField{Name: "inbox", CollectionId: mailboxes.Id, MaxSelect: 1})
	identities.Fields.Add(&core.TextField{Name: "email_name", Required: true})
	identities.Fields.Add(&core.TextField{Name: "default_from"})
	identities.Fields.Add(&core.BoolField{Name: "retired"})
	if err := app.Save(identities); err != nil {
		t.Fatal(err)
	}

	registerAddressHooks(app)
}
