import type { CoreStores } from '@tinycld/core/lib/pocketbase'
import type { Schema } from '@tinycld/core/types/pbSchema'
import type { createCollection } from 'pbtsdb/core'
import { BasicIndex } from 'pbtsdb/core'
import type { MailSchema } from './types'

// Replace (not intersect) the generated entries for mail's own collections:
// `Schema & MailSchema` would field-wise intersect each overlapping entry, and
// since the generated side types JSON columns as `any`, `any & Recipient[]`
// absorbs back to `any` — silently discarding every override in types.ts.
type MergedSchema = Omit<Schema, keyof MailSchema> & MailSchema

export function registerCollections(
    newCollection: ReturnType<typeof createCollection<MergedSchema>>,
    coreStores: CoreStores
) {
    // Hoisted, not inlined per-call: an inline `collectionOptions` object literal
    // defeats pbtsdb's inference of `alwaysFetchRelations` against `relations`,
    // making the expand keys resolve to `never`.
    const indexing = { autoIndex: 'eager' as const, defaultIndexType: BasicIndex }

    const mail_domains = newCollection('mail_domains', {
        omitOnInsert: ['created', 'updated', 'webhook_secret'] as const,
        collectionOptions: indexing,
    })

    // created_by is server-owned: mailbox_creator_guard.go sets it to the
    // caller, because the members bootstrap rule trusts it.
    const mail_mailboxes = newCollection('mail_mailboxes', {
        omitOnInsert: ['created', 'updated', 'created_by'] as const,
        relations: { domain: mail_domains },
        collectionOptions: indexing,
    })

    const mail_mailbox_members = newCollection('mail_mailbox_members', {
        omitOnInsert: ['created', 'updated'] as const,
        relations: { mailbox: mail_mailboxes, user: coreStores.users },
        collectionOptions: indexing,
    })

    const mail_threads = newCollection('mail_threads', {
        omitOnInsert: ['created', 'updated'] as const,
        // No `expand`: each on-demand thread fetch would otherwise carry
        // duplicate mail_mailboxes rows for every visible thread. The
        // mailbox is already loaded eagerly via mail_mailboxes; consumers
        // look it up by id (see useThreadListItems / useMailboxes).
        // Mailboxes can carry hundreds of thousands of threads. With on-demand,
        // each useLiveQuery against mail_threads translates its where/orderBy
        // into a PocketBase filter and runs server-side, so a folder view loads
        // only its visible page instead of the entire org's history.
        syncMode: 'on-demand' as const,
        collectionOptions: indexing,
    })

    const mail_mailbox_aliases = newCollection('mail_mailbox_aliases', {
        omitOnInsert: ['created', 'updated'] as const,
        relations: { mailbox: mail_mailboxes },
        collectionOptions: indexing,
    })

    const mail_messages = newCollection('mail_messages', {
        omitOnInsert: ['created', 'updated'] as const,
        // No `expand`: under on-demand mode each fetched message would
        // pull a copy of its parent mail_threads row plus the alias row.
        // Both are already loaded eagerly elsewhere (mail_threads via
        // useThreadListItems' page query, mail_mailbox_aliases as eager).
        // On-demand for the same reason as mail_threads: a power user can have
        // hundreds of thousands of message rows. Each useLiveQuery now runs
        // server-side filtered, e.g. messages for a single open thread.
        syncMode: 'on-demand' as const,
        collectionOptions: indexing,
    })

    const mail_thread_state = newCollection('mail_thread_state', {
        omitOnInsert: ['created', 'updated'] as const,
        // No `expand`, for the same reason as mail_threads and mail_messages:
        // under on-demand each fetched state row would carry a duplicate copy
        // of its thread and user. Both are already available — the thread from
        // useThreadListItems' page query, the user as the signed-in account.
        // A state row exists per (user, thread), so this collection grows with
        // the whole mailbox, not with what's on screen: an imported account
        // syncs tens of thousands of rows into the tab and eagerly indexes
        // them. Consumers now query only the thread ids they are rendering.
        syncMode: 'on-demand' as const,
        collectionOptions: indexing,
    })

    const mail_imap_mailbox_state = newCollection('mail_imap_mailbox_state', {
        omitOnInsert: ['created', 'updated'] as const,
        relations: { mailbox: mail_mailboxes },
        collectionOptions: indexing,
    })

    // Server-side aggregation of (user, mailbox) → folder counts. Backed by
    // a PocketBase view collection (see pb-migrations/1713000020). Eager: at
    // most one row per mailbox per user, used everywhere the sidebar renders.
    // No omitOnInsert — view collections are read-only.
    const mail_folder_counts = newCollection('mail_folder_counts', {
        relations: {
            user: coreStores.users,
            mailbox: mail_mailboxes,
        },
        collectionOptions: indexing,
    })

    return {
        mail_domains,
        mail_mailboxes,
        mail_mailbox_members,
        mail_mailbox_aliases,
        mail_threads,
        mail_messages,
        mail_thread_state,
        mail_imap_mailbox_state,
        mail_folder_counts,
    }
}
