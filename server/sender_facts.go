package mail

import (
	"context"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/addressbook"
)

// senderFacts are what the user already knows about a sender. InContacts is
// nil when the deployment has no address book: "unknown" must not reach the
// model or a rule as "not a contact".
type senderFacts struct {
	HasSentTo  bool
	InContacts *bool
}

func factsFor(ctx context.Context, app core.App, userID, senderEmail string) senderFacts {
	f := senderFacts{HasSentTo: hasSentTo(app, userID, senderEmail)}
	if known, ok := addressbook.IsKnown(ctx, app, userID, senderEmail); ok {
		f.InContacts = &known
	}
	return f
}

func (f senderFacts) evidence() map[string]any {
	out := map[string]any{"has_sent_to_sender": f.HasSentTo}
	if f.InContacts != nil {
		out["sender_in_contacts"] = *f.InContacts
	}
	return out
}

func (f senderFacts) key() string {
	c := "-"
	if f.InContacts != nil {
		c = fmt.Sprint(map[bool]int{false: 0, true: 1}[*f.InContacts])
	}
	return fmt.Sprintf("s%dc%s", map[bool]int{false: 0, true: 1}[f.HasSentTo], c)
}
