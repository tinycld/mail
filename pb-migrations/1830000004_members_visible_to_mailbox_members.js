/// <reference path="../../../server/pb_data/types.d.ts" />
// Make membership rows visible to the mailbox's members, and removable by its
// owners — the mail port of calendar's 1830000007.
//
// 1713000000 set mail_mailbox_members list/view/delete to the self-only
// `user = @request.auth.id` while create/update are owner-gated. That
// combination breaks the shared-mailbox roster in the drawer: an owner who
// adds a teammate sees the new row only optimistically (the local insert);
// after a reload the roster shows "1 member" again (the teammate's row exists
// but is invisible), and removing the teammate 404s because delete is
// self-only. Calendar hit and fixed the identical bug in 1830000007.
//
// list/view: membership rows are visible to every member of that mailbox —
// owners need the full roster to manage it, and a row reveals only
// (mailbox, user, role). Same back-relation shape the createRule already
// uses.
//
// delete: self-leave stays, and mailbox owners can remove any member's row —
// the exact predicate createRule's ownerCanAdd branch settled on.
//
// The disabled clause matches calendar's: a suspended user's still-live JWT
// must not keep reading or pruning rosters.
//
// Do NOT edit 1713000000 — it has shipped.
migrate(
    app => {
        const enabled = '@request.auth.disabled != true'
        const viaMember = 'mailbox.mail_mailbox_members_via_mailbox.user ?= @request.auth.id'
        const viaOwner =
            'mailbox.mail_mailbox_members_via_mailbox.user ?= @request.auth.id && ' +
            'mailbox.mail_mailbox_members_via_mailbox.role ?= "owner"'

        const members = app.findCollectionByNameOrId('mail_mailbox_members')
        members.listRule = `${enabled} && ${viaMember}`
        members.viewRule = `${enabled} && ${viaMember}`
        members.deleteRule = `${enabled} && (user = @request.auth.id || (${viaOwner}))`
        app.save(members)
    },
    app => {
        // Restore 1713000000's self-only rules.
        const members = app.findCollectionByNameOrId('mail_mailbox_members')
        members.listRule = 'user = @request.auth.id'
        members.viewRule = 'user = @request.auth.id'
        members.deleteRule = 'user = @request.auth.id'
        app.save(members)
    }
)
