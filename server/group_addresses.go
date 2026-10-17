package mail

import (
	"fmt"

	"github.com/pocketbase/dbx"
	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
)

// isGroupAddress reports whether an address delivers to a group (every
// member's own inbox) instead of one inbox.
func isGroupAddress(r *core.Record) bool {
	return r.GetString("group") != ""
}

// checkAddressTarget refuses a row that names both an inbox and a group, or
// neither. The schema cannot say "exactly one", so this hook does.
func checkAddressTarget(r *core.Record) error {
	hasMailbox := r.GetString("mailbox") != ""
	if hasMailbox && isGroupAddress(r) {
		return onField("group", validation.NewError("address_target_both", "Choose an inbox or a group, not both."))
	}
	if !hasMailbox && !isGroupAddress(r) {
		return onField("group", validation.NewError("address_target_missing", "Choose an inbox or a group."))
	}
	return nil
}

// registerGroupAddressHooks guards the rows that deliver to a group. Call it
// before registerAddressHooks: a row with no valid target is refused before
// the availability check names a conflict on it.
//
// A group address is always added by hand and is never a primary address:
// rule rows and primary rows belong to one person's inbox. via_group says
// which group gave a rule address, so it has no meaning here.
func registerGroupAddressHooks(app core.App) {
	guard := func(e *core.RecordEvent) error {
		if err := checkAddressTarget(e.Record); err != nil {
			return err
		}
		if isGroupAddress(e.Record) {
			e.Record.Set("source", addressSourceManual)
			e.Record.Set("is_primary", false)
			e.Record.Set("via_group", "")
		}
		return e.Next()
	}
	app.OnRecordCreate("mail_addresses").BindFunc(guard)
	app.OnRecordUpdate("mail_addresses").BindFunc(guard)
}

// deliveryTarget is one inbox a message is stored in, and the address
// (mail_addresses id) it came through, saved as the copy's delivered_to.
type deliveryTarget struct {
	Mailbox   *core.Record
	AddressID string
}

// deliveryTargets lists the inboxes mail to local@domain is stored in: the
// one inbox of an inbox address, or the own inbox of each group member with
// a live identity, ordered by user id. A group nobody can receive for is
// errAddressNotFound, the same as an address that does not exist, so the
// sender gets a bounce instead of the message being dropped. Other errors are
// database failures.
func deliveryTargets(app core.App, localPart, domainName string) ([]deliveryTarget, error) {
	address, domain, err := findAddress(app, localPart, domainName)
	if err != nil {
		return nil, err
	}
	if !isGroupAddress(address) {
		mailbox, err := inboxOfAddress(app, address, domain)
		if err != nil {
			return nil, err
		}
		return []deliveryTarget{{Mailbox: mailbox, AddressID: address.Id}}, nil
	}
	inboxes, err := groupMemberInboxes(app, address.GetString("group"))
	if err != nil {
		return nil, fmt.Errorf("inboxes of the members of %s: %w", addressText(address, domain), err)
	}
	if len(inboxes) == 0 {
		return nil, fmt.Errorf("%w: %s is a group with no member who can receive mail",
			errAddressNotFound, addressText(address, domain))
	}
	targets := make([]deliveryTarget, len(inboxes))
	for i, inbox := range inboxes {
		targets[i] = deliveryTarget{Mailbox: inbox, AddressID: address.Id}
	}
	addressLog.Debug("group address fan-out", "address", addressText(address, domain), "inboxes", len(targets))
	return targets, nil
}

// groupMemberInboxes returns the own inbox of each member of the group whose
// identity is live, ordered by user id. Two queries whatever the group's
// size: the back-relation keeps the filter one expression, and the inboxes
// load by id in one IN query.
func groupMemberInboxes(app core.App, groupID string) ([]*core.Record, error) {
	identities, err := app.FindRecordsByFilter("mail_identities",
		liveIdentityFilter+" && user.group_members_via_user.group ?= {:group}",
		"user", 0, 0, dbx.Params{"group": groupID})
	if err != nil {
		return nil, fmt.Errorf("list member identities: %w", err)
	}
	if len(identities) == 0 {
		return nil, nil
	}
	ids := make([]string, len(identities))
	for i, identity := range identities {
		ids[i] = identity.GetString("inbox")
	}
	found, err := app.FindRecordsByIds("mail_mailboxes", ids)
	if err != nil {
		return nil, fmt.Errorf("load member inboxes: %w", err)
	}
	byID := make(map[string]*core.Record, len(found))
	for _, inbox := range found {
		byID[inbox.Id] = inbox
	}
	inboxes := make([]*core.Record, 0, len(ids))
	for _, id := range ids {
		if inbox := byID[id]; inbox != nil {
			inboxes = append(inboxes, inbox)
		}
	}
	return inboxes, nil
}

// targetSet dedupes the inboxes of one message across all its recipients, so
// a person who is a direct recipient and on a list, or on two lists, gets one
// copy. It does not depend on the Message-ID, so a message without one is
// also stored once.
type targetSet struct {
	seen map[string]bool
}

// add reports whether t is the first target for its inbox. The first one is
// kept: recipients are walked in routing order, so delivered_to is the
// first address that matched.
func (s *targetSet) add(t deliveryTarget) bool {
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	if s.seen[t.Mailbox.Id] {
		return false
	}
	s.seen[t.Mailbox.Id] = true
	return true
}
