/// <reference path="../../../server/pb_data/types.d.ts" />
// SECURITY follow-up to 1830000002_exclude_guests_from_mail_infra.js.
//
// The `mail_mailbox_members` createRule contains a `bootstrapFirstOwner`
// sub-clause that lets an org member self-insert as the first owner of a
// memberless mailbox. The org-membership predicate
//   mailbox.domain.org.user_org_via_org.user ?= @request.auth.id
// does not exclude role='guest'. The transitive argument (a guest cannot
// create a mailbox after 1830000002, and every existing mailbox already has
// an owner) holds today, but a guest with a `user_org` row in the owning
// org could in principle race-bootstrap any mailbox that ever becomes
// memberless. Close the gap defensively by pinning
//   mailbox.domain.org.user_org_via_org.role ?!= "guest"
// to the SAME relation-path prefix as the user check, so PocketBase joins
// both predicates on the same user_org row (the CALLER's own membership
// must be non-guest). Verified against mail/server/guest_rls_test.go.
//
// The owner-adds-member branch is untouched: its existing
// `mailbox.mail_mailbox_members_via_mailbox.role ?= "owner"` pin already
// requires the caller to be a mailbox owner, and guests are never mailbox
// members at all.
migrate(
    app => {
        const ownerCanAdd =
            'mailbox.mail_mailbox_members_via_mailbox.user_org.user ?= @request.auth.id && mailbox.mail_mailbox_members_via_mailbox.role ?= "owner" && user_org.org = mailbox.domain.org'
        const bootstrapFirstOwner =
            'user_org.user = @request.auth.id && role = "owner" && mailbox.mail_mailbox_members_via_mailbox.id = "" && mailbox.domain.org.user_org_via_org.user ?= @request.auth.id && mailbox.domain.org.user_org_via_org.role ?!= "guest"'

        const col = app.findCollectionByNameOrId('mail_mailbox_members')
        col.createRule = `(${ownerCanAdd}) || (${bootstrapFirstOwner})`
        app.save(col)
    },
    app => {
        // Restore the EXACT prior createRule (from 1713000017 forward path).
        const ownerCanAdd =
            'mailbox.mail_mailbox_members_via_mailbox.user_org.user ?= @request.auth.id && mailbox.mail_mailbox_members_via_mailbox.role ?= "owner" && user_org.org = mailbox.domain.org'
        const bootstrapFirstOwner =
            'user_org.user = @request.auth.id && role = "owner" && mailbox.mail_mailbox_members_via_mailbox.id = "" && mailbox.domain.org.user_org_via_org.user ?= @request.auth.id'

        const col = app.findCollectionByNameOrId('mail_mailbox_members')
        col.createRule = `(${ownerCanAdd}) || (${bootstrapFirstOwner})`
        app.save(col)
    }
)
