/// <reference path="../../../server/pb_data/types.d.ts" />
// NOTE: this rule is mirrored in 1713000000_create_mail_collections.js. The
// fresh-install path applies the rule there directly; this migration upgrades
// existing installs. Any change to mail_mailbox_members.create must update
// BOTH files.
//
// Why the bootstrap clause exists: the original rule required the requester to
// already be an owner of the mailbox. That made it impossible to add the very
// first owner — a freshly-created mailbox has zero members, so the
// `mailbox.mail_mailbox_members_via_mailbox` chain matches nothing. The new
// clause permits a single self-as-owner insert when the mailbox has no members
// yet AND the requester belongs to the owning org.
migrate(
    app => {
        const ownerCanAdd =
            'mailbox.mail_mailbox_members_via_mailbox.user_org.user ?= @request.auth.id && mailbox.mail_mailbox_members_via_mailbox.role ?= "owner"'
        const bootstrapFirstOwner =
            'role = "owner" && mailbox.mail_mailbox_members_via_mailbox.id = "" && mailbox.domain.org.user_org_via_org.user ?= @request.auth.id'

        const col = app.findCollectionByNameOrId('mail_mailbox_members')
        col.createRule = `user_org.user = @request.auth.id && ((${ownerCanAdd}) || (${bootstrapFirstOwner}))`
        app.save(col)
    },
    app => {
        const col = app.findCollectionByNameOrId('mail_mailbox_members')
        col.createRule =
            'user_org.user = @request.auth.id && mailbox.mail_mailbox_members_via_mailbox.user_org.user ?= @request.auth.id && mailbox.mail_mailbox_members_via_mailbox.role ?= "owner"'
        app.save(col)
    }
)
