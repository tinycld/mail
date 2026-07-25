/// <reference path="../../../server/pb_data/types.d.ts" />
// NOTE: this rule is mirrored in 1713000000_create_mail_collections.js. The
// fresh-install path applies the rule there directly; this migration upgrades
// existing installs. Any change to mail_mailbox_members.create must update
// BOTH files.
//
// Why this fix exists: the previous rule wrapped both the owner-adds-member
// path and the bootstrap path with `user = @request.auth.id`, which
// only makes sense for the bootstrap (self-insert as first owner). When an
// owner adds a teammate, `user` on the new row is the teammate's, not the
// requester's, so the outer constraint always failed. Move the requester-self
// check inside the bootstrap branch.
migrate(
    app => {
        const ownerCanAdd =
            'mailbox.mail_mailbox_members_via_mailbox.user ?= @request.auth.id && mailbox.mail_mailbox_members_via_mailbox.role ?= "owner"'
        const bootstrapFirstOwner =
            'user = @request.auth.id && role = "owner" && mailbox.mail_mailbox_members_via_mailbox.id = ""'

        const col = app.findCollectionByNameOrId('mail_mailbox_members')
        col.createRule = `(${ownerCanAdd}) || (${bootstrapFirstOwner})`
        app.save(col)
    },
    app => {
        const ownerCanAdd =
            'mailbox.mail_mailbox_members_via_mailbox.user ?= @request.auth.id && mailbox.mail_mailbox_members_via_mailbox.role ?= "owner"'
        const bootstrapFirstOwner =
            'role = "owner" && mailbox.mail_mailbox_members_via_mailbox.id = ""'

        const col = app.findCollectionByNameOrId('mail_mailbox_members')
        col.createRule = `user = @request.auth.id && ((${ownerCanAdd}) || (${bootstrapFirstOwner}))`
        app.save(col)
    }
)
