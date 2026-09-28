import type { CoreStores } from '@tinycld/core/lib/pocketbase'
import type { createCollection } from 'pbtsdb/core'
import { describe, expect, it, vi } from 'vitest'
import { registerCollections } from '../tinycld/mail/collections'
import type { MailSchema } from '../tinycld/mail/types'

// Every mail collection must be on-demand with per-query realtime (pbtsdb
// 0.10): only the rows a live query asks for should ever enter the store or
// get subscribed to. This test pins that contract so a future collection
// added to collections.ts without `...onDemand` fails loudly instead of
// quietly reverting to an eager, whole-collection sync.
const EXPECTED_COLLECTION_NAMES = [
    'mail_domains',
    'mail_mailboxes',
    'mail_mailbox_members',
    'mail_mailbox_aliases',
    'mail_threads',
    'mail_messages',
    'mail_thread_state',
    'mail_imap_mailbox_state',
    'mail_folder_counts',
]

describe('mail collections', () => {
    it('registers every collection as on-demand with per-query realtime', () => {
        // Cast once, at the boundary: registerCollections' signature demands
        // pbtsdb's real (heavily generic, curried) createCollection type, but
        // this test only needs to record each call's (name, options) pair.
        const spy = vi.fn((name: string, options: Record<string, unknown>) => ({
            __name: name,
            __options: options,
        }))
        const spyNewCollection = spy as unknown as ReturnType<typeof createCollection<MailSchema>>

        const fakeCoreStores = { users: { __name: 'users' } } as unknown as CoreStores

        const collections = registerCollections(spyNewCollection, fakeCoreStores)

        expect(Object.keys(collections).sort()).toEqual([...EXPECTED_COLLECTION_NAMES].sort())
        expect(spy.mock.calls.map(([name]) => name).sort()).toEqual(
            [...EXPECTED_COLLECTION_NAMES].sort()
        )

        for (const [name, options] of spy.mock.calls) {
            expect(options, `${name} options`).toMatchObject({
                syncMode: 'on-demand',
                realtime: 'query',
            })
        }
    })
})
