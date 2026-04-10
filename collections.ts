import type { createCollection } from 'pbtsdb'
import { BasicIndex } from 'pbtsdb'
import type { CoreStores } from '~/lib/pocketbase'
import type { Schema } from '~/types/pbSchema'
import type { MailSchema } from './types'

type MergedSchema = Schema & MailSchema

export function registerCollections(
    newCollection: ReturnType<typeof createCollection<MergedSchema>>,
    coreStores: CoreStores
) {
    const mail_domains = newCollection('mail_domains', {
        omitOnInsert: ['created', 'updated'] as const,
        collectionOptions: {
            autoIndex: 'eager' as const,
            defaultIndexType: BasicIndex,
        },
    })

    const mail_mailboxes = newCollection('mail_mailboxes', {
        omitOnInsert: ['created', 'updated'] as const,
        expand: { domain: mail_domains },
        collectionOptions: {
            autoIndex: 'eager' as const,
            defaultIndexType: BasicIndex,
        },
    })

    const mail_mailbox_members = newCollection('mail_mailbox_members', {
        omitOnInsert: ['created', 'updated'] as const,
        expand: { mailbox: mail_mailboxes, user_org: coreStores.user_org },
        collectionOptions: {
            autoIndex: 'eager' as const,
            defaultIndexType: BasicIndex,
        },
    })

    const mail_threads = newCollection('mail_threads', {
        omitOnInsert: ['created', 'updated'] as const,
        expand: { mailbox: mail_mailboxes },
        collectionOptions: {
            autoIndex: 'eager' as const,
            defaultIndexType: BasicIndex,
        },
    })

    const mail_messages = newCollection('mail_messages', {
        omitOnInsert: ['created', 'updated'] as const,
        expand: { thread: mail_threads },
        collectionOptions: {
            autoIndex: 'eager' as const,
            defaultIndexType: BasicIndex,
        },
    })

    const mail_thread_state = newCollection('mail_thread_state', {
        omitOnInsert: ['created', 'updated'] as const,
        expand: {
            thread: mail_threads,
            user_org: coreStores.user_org,
        },
        collectionOptions: {
            autoIndex: 'eager' as const,
            defaultIndexType: BasicIndex,
        },
    })

    const mail_imap_mailbox_state = newCollection('mail_imap_mailbox_state', {
        omitOnInsert: ['created', 'updated'] as const,
        expand: { mailbox: mail_mailboxes },
        collectionOptions: {
            autoIndex: 'eager' as const,
            defaultIndexType: BasicIndex,
        },
    })

    return {
        mail_domains,
        mail_mailboxes,
        mail_mailbox_members,
        mail_threads,
        mail_messages,
        mail_thread_state,
        mail_imap_mailbox_state,
    }
}
