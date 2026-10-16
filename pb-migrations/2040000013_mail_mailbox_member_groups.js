/// <reference path="../pb_data/types.d.ts" />
// A group can be a member of a shared inbox (design 2026-10-15, phase 3B).
// mail_mailbox_members becomes a core groups grant table
// (server/mailbox_member_groups.go registers it):
//   - a person row has `user` set and `group` empty, as before;
//   - a GRANT row has `group` set and `user` empty. Only an owner of a shared
//     inbox writes one, and its role is always member;
//   - a DERIVED row has both set. Core's expander writes one per group member
//     and keeps them in step with the group. The request hooks refuse every
//     client write to a derived row, with a message that names the group.
//
// Named after core's 2040000000_create_groups so pbc_groups_01 exists when this
// runs. cascadeDelete on `group`: deleting a group removes its grant and
// derived rows.
//
// The unique index now spans (mailbox, user, group), which core's expander
// needs: one person row per user, one grant per group, and one derived row per
// (group, user) on an inbox.
//
// Rules. list/view stay as 2040000012 left them, update/delete as 1830000007
// left them. PocketBase loads a record through the update or delete rule
// before the request hooks run, so the rules must still let a derived row's
// own user (delete) and an owner (update, delete) reach the hooks that refuse
// derived rows. A grant row stores `user` as "", so it matches only the
// owner branch of those rules: only an owner changes or removes a grant. Every
// clause that tests `user` keeps the login guard (see 1830000007 and core's
// groups package: with no login, `user ?= @request.auth.id` matches "").
//
// The create rule changes: an owner adds a person (no group) or grants a group
// on a shared inbox (no user, role member), and the first-owner bootstrap
// cannot carry a group.

// Copied verbatim from 1830000007_require_login_on_mailbox_rules.js.
const authed = '@request.auth.id != ""'
const enabled = '@request.auth.disabled != true'
const viaMember = 'mailbox.mail_mailbox_members_via_mailbox.user ?= @request.auth.id'
const viaOwner = `${viaMember} && mailbox.mail_mailbox_members_via_mailbox.role ?= "owner"`
const bootstrapFirstOwner =
    'user = @request.auth.id && role = "owner" && mailbox.mail_mailbox_members_via_mailbox.id = "" && mailbox.created_by = @request.auth.id && @request.auth.role != "guest"'

// From 2040000012_mail_admin_reads_inboxes.js.
const admin = '(@request.auth.role = "admin" || @request.auth.role = "owner")'

const oldUniqueIndex = 'CREATE UNIQUE INDEX `idx_mail_mb_members_unique` ON `mail_mailbox_members` (`mailbox`, `user`)'
const newUniqueIndex = 'CREATE UNIQUE INDEX `idx_mail_mb_members_unique` ON `mail_mailbox_members` (`mailbox`, `user`, `group`)'
const otherIndexes = [
    'CREATE INDEX `idx_mail_mb_members_user` ON `mail_mailbox_members` (`user`)',
    'CREATE INDEX `idx_mail_mb_members_mailbox` ON `mail_mailbox_members` (`mailbox`)',
]

const readRule = `${authed} && ${enabled} && (${viaMember} || ${admin})`
const updateRule = `${authed} && ${viaOwner}`
const deleteRule = `${authed} && ${enabled} && (user = @request.auth.id || (${viaOwner}))`

migrate(
    app => {
        const members = app.findCollectionByNameOrId('mail_mailbox_members')
        members.fields.getById('mail_mb_members_user').required = false
        members.fields.add(
            new Field({
                id: 'mail_mb_members_group',
                name: 'group',
                type: 'relation',
                collectionId: 'pbc_groups_01',
                cascadeDelete: true,
                maxSelect: 1,
                required: false,
            })
        )
        members.indexes = [newUniqueIndex, ...otherIndexes]
        members.listRule = readRule
        members.viewRule = readRule
        members.createRule = `${authed} && (
            (${viaOwner} && ((group = "" && user != "") || (user = "" && group != "" && role = "member" && mailbox.type = "shared")))
            || (${bootstrapFirstOwner} && group = "")
        )`
        members.updateRule = updateRule
        members.deleteRule = deleteRule
        app.save(members)
    },
    app => {
        // Grant and derived rows cannot exist without the group field, and a
        // grant's empty user would fail the restored required check.
        app.db().newQuery(`DELETE FROM mail_mailbox_members WHERE user = '' OR "group" != ''`).execute()

        const members = app.findCollectionByNameOrId('mail_mailbox_members')
        members.indexes = [oldUniqueIndex, ...otherIndexes]
        members.listRule = readRule
        members.viewRule = readRule
        members.createRule = `${authed} && ((${viaOwner}) || (${bootstrapFirstOwner}))`
        members.updateRule = updateRule
        members.deleteRule = deleteRule
        members.fields.removeById('mail_mb_members_group')
        members.fields.getById('mail_mb_members_user').required = true
        app.save(members)
    }
)
