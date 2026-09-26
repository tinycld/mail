package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/rlstest"
)

// TestMailMessagesHasDeliveryTrackingFields proves the shipped migrations add
// delivered_at and bounce_class to mail_messages with the right types and
// select values, so later provider-notification code has somewhere to write.
func TestMailMessagesHasDeliveryTrackingFields(t *testing.T) {
	app := rlstest.NewApp(t)

	// 1830000007's mail_mailbox_members rules read users.role/disabled, which
	// belong to core's users schema and aren't carried by the bare test app.
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	users.Fields.Add(&core.SelectField{
		Name: "role", MaxSelect: 1,
		Values: []string{"owner", "admin", "member", "guest"},
	})
	users.Fields.Add(&core.BoolField{Name: "disabled"})
	if err := app.Save(users); err != nil {
		t.Fatalf("add users fields: %v", err)
	}

	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb-migrations"))

	collection, err := app.FindCollectionByNameOrId("mail_messages")
	if err != nil {
		t.Fatalf("find mail_messages: %v", err)
	}

	deliveredAt := collection.Fields.GetByName("delivered_at")
	if deliveredAt == nil {
		t.Fatal("mail_messages missing delivered_at field")
	}
	if _, ok := deliveredAt.(*core.DateField); !ok {
		t.Errorf("delivered_at type = %T, want *core.DateField", deliveredAt)
	}

	bounceClass := collection.Fields.GetByName("bounce_class")
	if bounceClass == nil {
		t.Fatal("mail_messages missing bounce_class field")
	}
	selectField, ok := bounceClass.(*core.SelectField)
	if !ok {
		t.Fatalf("bounce_class type = %T, want *core.SelectField", bounceClass)
	}
	if selectField.Required {
		t.Error("bounce_class should not be required")
	}
	wantValues := []string{"soft", "hard", "complaint"}
	if len(selectField.Values) != len(wantValues) {
		t.Fatalf("bounce_class values = %v, want %v", selectField.Values, wantValues)
	}
	for i, v := range wantValues {
		if selectField.Values[i] != v {
			t.Errorf("bounce_class values[%d] = %q, want %q", i, selectField.Values[i], v)
		}
	}
}
