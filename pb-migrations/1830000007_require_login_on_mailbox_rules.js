/// <reference path="../../../server/pb_data/types.d.ts" />
// SECURITY: require a login on every mail rule that reads @request.auth.
//
// With no login, PocketBase resolves @request.auth.id to NULL, and its filter
// compiler turns `x = NULL` into `(x = '' OR x IS NULL)`. A back-relation such
// as `mail_mailbox_members_via_mailbox.user ?= @request.auth.id` is a LEFT
// JOIN, so a mailbox with ZERO member rows yields NULL and matches the
// anonymous caller. A shared mailbox has zero members once its last member
// deletes their account (lifecycle.go sweeps only personal mailboxes). An
// anonymous caller could then list, view, create, update and delete that
// mailbox's threads and messages, and a realtime subscription received its
// new mail (realtime checks the same list/view rules).
//
// Negative-only checks do not help: `@request.auth.role != "guest"` and
// `@request.auth.disabled != true` are TRUE for an anonymous caller. Only
// `@request.auth.id != ""` compiles to false without a login.
//
// So every mail rule that reads @request.auth now opens with that guard,
// including the ones that are safe today because their FK is required
// (mail_thread_state, mail_folder_counts, mail_mailbox_members): a later
// schema change must not silently reopen them. mail_domains,
// mail_mailbox_aliases and mail_mailboxes.create already carry the guard.
//
// OWNER CLAIM. The mail_mailbox_members bootstrap branch lets a user add
// themselves as the first owner of a mailbox with no members, so the user who
// creates a shared mailbox can own it. It checked nothing else, so any
// logged-in non-guest could make themselves owner of a shared mailbox whose
// last member had deleted their account, and then read and send its mail.
// mail_mailboxes now records created_by, and the bootstrap branch also
// requires `mailbox.created_by = @request.auth.id`. The Go hook in
// mailbox_creator_guard.go sets created_by to the caller on every API create
// and keeps it on every API update, so a client cannot choose it. Released
// clients send no created_by, so a rule that required it in the body would
// break shared-mailbox creation for them; the hook does not. created_by is
// optional and does not cascade: when the creator's account is deleted it
// clears, the mailbox and its mail stay, and nobody can bootstrap it again (an
// operator repairs that state). Mailboxes that exist before this migration
// have no created_by, so the bootstrap branch never applies to them; each one
// already had its first owner.
//
// This file is edited in place for the owner-claim fix because it has not
// been released: it ships in the same unreleased mail version as the login
// guard, so no database has applied an older form of it.
//
// Rules are restated as literals, as 1830000003 / 1830000004 do. The down
// migration restores the exact prior strings. Verified against the real rule
// engine in mail/server/anon_rule_guard_test.go and owner_claim_test.go.
//
// Do NOT edit released migrations (1830000006 and earlier) — mail has shipped.
migrate(
    app => {
        const authed = '@request.auth.id != ""'
        const enabled = '@request.auth.disabled != true'

        const mbMember = 'mail_mailbox_members_via_mailbox.user ?= @request.auth.id'
        const mbOwner = `${mbMember} && mail_mailbox_members_via_mailbox.role ?= "owner"`
        const mailboxes = app.findCollectionByNameOrId('mail_mailboxes')
        mailboxes.fields.add(
            new Field({
                id: 'mail_mailboxes_created_by',
                name: 'created_by',
                type: 'relation',
                required: false,
                collectionId: app.findCollectionByNameOrId('users').id,
                cascadeDelete: false,
                maxSelect: 1,
            })
        )
        mailboxes.listRule = `${authed} && ${mbMember}`
        mailboxes.viewRule = `${authed} && ${mbMember}`
        mailboxes.updateRule = `${authed} && ${mbOwner}`
        mailboxes.deleteRule = `${authed} && ${mbOwner}`
        app.save(mailboxes)

        const viaMember = 'mailbox.mail_mailbox_members_via_mailbox.user ?= @request.auth.id'
        const viaOwner = `${viaMember} && mailbox.mail_mailbox_members_via_mailbox.role ?= "owner"`
        const bootstrapFirstOwner =
            'user = @request.auth.id && role = "owner" && mailbox.mail_mailbox_members_via_mailbox.id = "" && mailbox.created_by = @request.auth.id && @request.auth.role != "guest"'
        const members = app.findCollectionByNameOrId('mail_mailbox_members')
        members.listRule = `${authed} && ${enabled} && ${viaMember}`
        members.viewRule = `${authed} && ${enabled} && ${viaMember}`
        members.createRule = `${authed} && ((${viaOwner}) || (${bootstrapFirstOwner}))`
        members.updateRule = `${authed} && ${viaOwner}`
        members.deleteRule = `${authed} && ${enabled} && (user = @request.auth.id || (${viaOwner}))`
        app.save(members)

        const threads = app.findCollectionByNameOrId('mail_threads')
        const threadRule = `${authed} && ${viaMember}`
        threads.listRule = threadRule
        threads.viewRule = threadRule
        threads.createRule = threadRule
        threads.updateRule = threadRule
        threads.deleteRule = threadRule
        app.save(threads)

        const messages = app.findCollectionByNameOrId('mail_messages')
        const messageRule = `${authed} && thread.${viaMember}`
        messages.listRule = messageRule
        messages.viewRule = messageRule
        messages.createRule = messageRule
        messages.updateRule = messageRule
        messages.deleteRule = messageRule
        app.save(messages)

        const own = 'user = @request.auth.id'
        const state = app.findCollectionByNameOrId('mail_thread_state')
        state.listRule = `${authed} && ${own}`
        state.viewRule = `${authed} && ${own}`
        state.createRule = `${authed} && ${own} && thread.${viaMember}`
        state.updateRule = `${authed} && ${own}`
        state.deleteRule = `${authed} && ${own}`
        app.save(state)

        const counts = app.findCollectionByNameOrId('mail_folder_counts')
        counts.listRule = `${authed} && user ?= @request.auth.id`
        counts.viewRule = `${authed} && user ?= @request.auth.id`
        app.save(counts)
    },
    app => {
        // Restore the exact prior rules (1713000000 / 1713000017 / 1830000000
        // / 1830000003 / 1830000004).
        const enabled = '@request.auth.disabled != true'

        const mbMember = 'mail_mailbox_members_via_mailbox.user ?= @request.auth.id'
        const mbOwner = `${mbMember} && mail_mailbox_members_via_mailbox.role ?= "owner"`
        const mailboxes = app.findCollectionByNameOrId('mail_mailboxes')
        mailboxes.listRule = mbMember
        mailboxes.viewRule = mbMember
        mailboxes.updateRule = mbOwner
        mailboxes.deleteRule = mbOwner
        app.save(mailboxes)

        const viaMember = 'mailbox.mail_mailbox_members_via_mailbox.user ?= @request.auth.id'
        const viaOwner = `${viaMember} && mailbox.mail_mailbox_members_via_mailbox.role ?= "owner"`
        const bootstrapFirstOwner =
            'user = @request.auth.id && role = "owner" && mailbox.mail_mailbox_members_via_mailbox.id = "" && @request.auth.role != "guest"'
        const members = app.findCollectionByNameOrId('mail_mailbox_members')
        members.listRule = `${enabled} && ${viaMember}`
        members.viewRule = `${enabled} && ${viaMember}`
        members.createRule = `(${viaOwner}) || (${bootstrapFirstOwner})`
        members.updateRule = viaOwner
        members.deleteRule = `${enabled} && (user = @request.auth.id || (${viaOwner}))`
        app.save(members)

        const threads = app.findCollectionByNameOrId('mail_threads')
        threads.listRule = viaMember
        threads.viewRule = viaMember
        threads.createRule = viaMember
        threads.updateRule = viaMember
        threads.deleteRule = viaMember
        app.save(threads)

        const messages = app.findCollectionByNameOrId('mail_messages')
        const messageRule = `thread.${viaMember}`
        messages.listRule = messageRule
        messages.viewRule = messageRule
        messages.createRule = messageRule
        messages.updateRule = messageRule
        messages.deleteRule = messageRule
        app.save(messages)

        const own = 'user = @request.auth.id'
        const state = app.findCollectionByNameOrId('mail_thread_state')
        state.listRule = own
        state.viewRule = own
        state.createRule = `${own} && thread.${viaMember}`
        state.updateRule = own
        state.deleteRule = own
        app.save(state)

        const counts = app.findCollectionByNameOrId('mail_folder_counts')
        counts.listRule = 'user ?= @request.auth.id'
        counts.viewRule = 'user ?= @request.auth.id'
        app.save(counts)

        // Removed last: the members createRule restored above no longer reads it.
        const mailboxesAgain = app.findCollectionByNameOrId('mail_mailboxes')
        mailboxesAgain.fields.removeById('mail_mailboxes_created_by')
        app.save(mailboxesAgain)
    }
)
