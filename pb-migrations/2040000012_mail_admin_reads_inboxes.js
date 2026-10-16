/// <reference path="../pb_data/types.d.ts" />
// An admin or owner reads every shared inbox and every member row, so the
// Shared inboxes screen and the Members → Email panel list all of them. This
// is list and view only: threads, messages, thread state and folder counts
// keep their member-only rules, and no write rule changes.
//
// Every branch opens with the login guard: with no login the member
// back-relation compiles to a NULL match (see 1830000007).
const authed = '@request.auth.id != ""'
const enabled = '@request.auth.disabled != true'
const admin = '(@request.auth.role = "admin" || @request.auth.role = "owner")'

migrate(
    app => {
        const mbMember = 'mail_mailbox_members_via_mailbox.user ?= @request.auth.id'
        const mailboxes = app.findCollectionByNameOrId('mail_mailboxes')
        mailboxes.listRule = `${authed} && ${enabled} && (${mbMember} || ${admin})`
        mailboxes.viewRule = `${authed} && ${enabled} && (${mbMember} || ${admin})`
        app.save(mailboxes)

        const viaMember = 'mailbox.mail_mailbox_members_via_mailbox.user ?= @request.auth.id'
        const members = app.findCollectionByNameOrId('mail_mailbox_members')
        members.listRule = `${authed} && ${enabled} && (${viaMember} || ${admin})`
        members.viewRule = `${authed} && ${enabled} && (${viaMember} || ${admin})`
        app.save(members)
    },
    app => {
        const mbMember = 'mail_mailbox_members_via_mailbox.user ?= @request.auth.id'
        const mailboxes = app.findCollectionByNameOrId('mail_mailboxes')
        mailboxes.listRule = `${authed} && ${mbMember}`
        mailboxes.viewRule = `${authed} && ${mbMember}`
        app.save(mailboxes)

        const viaMember = 'mailbox.mail_mailbox_members_via_mailbox.user ?= @request.auth.id'
        const members = app.findCollectionByNameOrId('mail_mailbox_members')
        members.listRule = `${authed} && ${enabled} && ${viaMember}`
        members.viewRule = `${authed} && ${enabled} && ${viaMember}`
        app.save(members)
    }
)
