/// <reference path="../../../server/pb_data/types.d.ts" />
// SECURITY: exclude the 'guest' role from mail-infra access rules.
//
// A guest share-link visitor gets a real users record + a user_org row with
// role='guest' in the owner's org. Several mail-infra rules granted access to
// ANY org member regardless of role:
//   - mail_domains          list/view  (read leak: guest enumerates domains)
//   - mail_labels           all CRUD   (read + write leak)
//   - mail_mailboxes        create     (write leak: guest creates a mailbox)
//   - mail_mailbox_aliases  list/view  (read leak: guest enumerates aliases)
//
// Each tightened rule pins the role check (`role ?!= "guest"`) to the SAME
// relation-path prefix as the user check, so PocketBase applies both to the
// same joined user_org row (the CALLER's own membership must be non-guest —
// verified against the real rule engine in mail/server/guest_rls_test.go).
//
// Rules deliberately LEFT UNCHANGED because they're already safe for guests:
//   - mail_domains create/update/delete + mail_mailbox_aliases create/update/
//     delete are admin/owner-gated (a guest is neither).
//   - mail_mailboxes list/view is mailbox-membership-gated (a guest is not a
//     mailbox member).
//   - mail_threads / mail_messages / mail_thread_state are mailbox-membership
//     gated, never org-membership gated — a guest holds no mailbox membership.
//
// DEFERRED (documented, lower severity): mail_mailbox_members.create has a
// `bootstrapFirstOwner` branch gated on org membership
// (`mailbox.domain.org.user_org_via_org.user ?= @request.auth.id`) that, in
// isolation, would let a guest self-insert as first owner of a *memberless*
// mailbox. In practice this is blocked transitively — a guest cannot create a
// mailbox (tightened above) and every existing mailbox already has an owner
// (so `...mail_mailbox_members_via_mailbox.id = ""` is false). Tightening it
// cleanly requires editing the compound rule mirrored across
// 1713000000 (phase-2) and 1713000017; left for a focused follow-up.
//
// The down-migration restores the EXACT prior rule strings (mail_domains from
// 1713000011; mail_labels + mail_mailboxes.create from 1713000000 phase-2;
// mail_mailbox_aliases from 1713000014).
migrate(
    app => {
        const orgScopedRule =
            'org.user_org_via_org.user ?= @request.auth.id && ' +
            'org.user_org_via_org.role ?!= "guest"'

        // mail_domains: tighten READ only (writes already admin/owner-gated).
        const domains = app.findCollectionByNameOrId('mail_domains')
        domains.listRule = orgScopedRule
        domains.viewRule = orgScopedRule
        app.save(domains)

        // mail_labels: tighten all five (was org-member for everything).
        const labels = app.findCollectionByNameOrId('mail_labels')
        labels.listRule = orgScopedRule
        labels.viewRule = orgScopedRule
        labels.createRule = orgScopedRule
        labels.updateRule = orgScopedRule
        labels.deleteRule = orgScopedRule
        app.save(labels)

        // mail_mailboxes: tighten CREATE only (read is mailbox-member-gated;
        // update/delete are owner-gated).
        const mailboxesCreateRule =
            'domain.org.user_org_via_org.user ?= @request.auth.id && ' +
            'domain.org.user_org_via_org.role ?!= "guest"'
        const mailboxes = app.findCollectionByNameOrId('mail_mailboxes')
        mailboxes.createRule = mailboxesCreateRule
        app.save(mailboxes)

        // mail_mailbox_aliases: tighten READ only (writes already admin/owner).
        const aliasesReadRule =
            '@request.auth.id != "" && ' +
            'mailbox.domain.org.user_org_via_org.user ?= @request.auth.id && ' +
            'mailbox.domain.org.user_org_via_org.role ?!= "guest"'
        const aliases = app.findCollectionByNameOrId('mail_mailbox_aliases')
        aliases.listRule = aliasesReadRule
        aliases.viewRule = aliasesReadRule
        app.save(aliases)
    },
    app => {
        // Restore EXACT prior rule strings.
        const orgMemberRule = 'org.user_org_via_org.user ?= @request.auth.id'

        // mail_domains (memberRule from 1713000011 / 1713000000)
        const domains = app.findCollectionByNameOrId('mail_domains')
        domains.listRule = orgMemberRule
        domains.viewRule = orgMemberRule
        app.save(domains)

        // mail_labels (orgMemberRule from 1713000000)
        const labels = app.findCollectionByNameOrId('mail_labels')
        labels.listRule = orgMemberRule
        labels.viewRule = orgMemberRule
        labels.createRule = orgMemberRule
        labels.updateRule = orgMemberRule
        labels.deleteRule = orgMemberRule
        app.save(labels)

        // mail_mailboxes create (from 1713000000)
        const mailboxes = app.findCollectionByNameOrId('mail_mailboxes')
        mailboxes.createRule = 'domain.org.user_org_via_org.user ?= @request.auth.id'
        app.save(mailboxes)

        // mail_mailbox_aliases (from 1713000014)
        const aliasesPrior =
            '@request.auth.id != "" && mailbox.domain.org.user_org_via_org.user ?= @request.auth.id'
        const aliases = app.findCollectionByNameOrId('mail_mailbox_aliases')
        aliases.listRule = aliasesPrior
        aliases.viewRule = aliasesPrior
        app.save(aliases)
    }
)
