// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

// No vitest globals in this workspace, so testing-library's automatic
// afterEach cleanup never registers.
afterEach(cleanup)

// The hook joins a live row (the draft message) with a file fetched over
// HTTP (the body). This mounts the real hook against a real TanStack DB
// collection so the thread/delivery_status predicate actually executes, and
// stubs only the file-token + fetch boundary that a unit test cannot cross.
const h = vi.hoisted(() => ({
    messages: [
        {
            id: 'msg_draft1',
            thread: 'thread1',
            delivery_status: 'draft',
            subject: 'Draft subject',
            snippet: 'fallback snippet',
            body_html: 'body.html',
            alias: 'alias1',
            recipients_to: [{ name: '', email: 'to@acme.com' }],
            recipients_cc: [],
            recipients_bcc: [],
            created: '',
            updated: '',
        },
        // A sent message on the same thread — must never be picked as the draft.
        {
            id: 'msg_sent1',
            thread: 'thread1',
            delivery_status: 'sent',
            subject: 'Sent subject',
            snippet: '',
            body_html: '',
            alias: '',
            recipients_to: [],
            recipients_cc: [],
            recipients_bcc: [],
            created: '',
            updated: '',
        },
        // Another thread's draft — must never leak into this thread's result.
        {
            id: 'msg_draft2',
            thread: 'thread2',
            delivery_status: 'draft',
            subject: 'Other draft',
            snippet: '',
            body_html: '',
            alias: '',
            recipients_to: [],
            recipients_cc: [],
            recipients_bcc: [],
            created: '',
            updated: '',
        },
    ],
}))

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { BasicIndex, createCollection, localOnlyCollectionOptions } = await import(
        '@tanstack/db'
    )
    const messages = createCollection({
        ...localOnlyCollectionOptions({
            id: 'mail_messages',
            getKey: (r: { id: string }) => r.id,
            initialData: h.messages,
        }),
        autoIndex: 'eager',
        defaultIndexType: BasicIndex,
    })
    return {
        useStore: () => [messages],
        pb: {
            files: {
                getToken: vi.fn().mockResolvedValue('tok_123'),
                getURL: vi.fn(
                    (record: { id: string }, filename: string) =>
                        `https://pb.test/api/files/mail_messages/${record.id}/${filename}`
                ),
            },
        },
    }
})

vi.mock('@tinycld/core/lib/server-fetch', () => ({
    serverFetch: vi.fn().mockResolvedValue({ text: () => Promise.resolve('<p>hello</p>') }),
}))

import { useDraftMessage } from '~/tinycld/mail/hooks/useDraftMessage'

const withQueryClient = (ui: ReactNode) => (
    <QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>
)

test('returns null until the draft row and body resolve, then the message and html body', async () => {
    const { result } = renderHook(
        () => useDraftMessage({ threadId: 'thread1', mailboxId: 'mb1' }),
        { wrapper: ({ children }) => withQueryClient(children) }
    )

    expect(result.current).toBeNull()

    await waitFor(() => expect(result.current).not.toBeNull())

    expect(result.current?.message.id).toBe('msg_draft1')
    expect(result.current?.htmlBody).toBe('<p>hello</p>')
})

test('returns null when the context has no thread id', () => {
    const { result } = renderHook(() => useDraftMessage(null), {
        wrapper: ({ children }) => withQueryClient(children),
    })

    expect(result.current).toBeNull()
})

// Opening a second draft (e.g. clicking draft B while draft A is shown)
// swaps draftContext to a thread whose draft row has not synced in yet.
// The hook must drop A's resolved result immediately rather than hold it
// stale until B resolves — the consuming component's effect relies on this
// to clear the form instead of leaving A's content on screen.
test('returns null again when the thread id switches to one with no draft in the store', async () => {
    const { result, rerender } = renderHook(
        ({ threadId }: { threadId: string }) => useDraftMessage({ threadId, mailboxId: 'mb1' }),
        {
            wrapper: ({ children }) => withQueryClient(children),
            initialProps: { threadId: 'thread1' },
        }
    )

    await waitFor(() => expect(result.current?.message.id).toBe('msg_draft1'))

    rerender({ threadId: 'thread_missing' })

    expect(result.current).toBeNull()
})
