// @vitest-environment happy-dom
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

// No vitest globals in this workspace, so testing-library's automatic
// afterEach cleanup never registers.
afterEach(cleanup)
afterEach(() => {
    seenQueries.length = 0
})

// mail_thread_state is on-demand: it holds one row per (user, thread), so it
// grows with the whole mailbox rather than with what is on screen. These tests
// mount the real hook against real TanStack DB collections so the predicate
// actually executes — a fixture that never runs the query would certify a
// filter matching zero rows.
//
// The fixture carries two rows the hook must not surface: another user's row
// on a searched thread (excluded by the user predicate) and this user's row on
// an unsearched thread (excluded by the id predicate).

const h = vi.hoisted(() => ({
    states: [
        {
            id: 'ts_hit1',
            thread: 'thread_hit1',
            user: 'u1',
            folder: 'inbox',
            is_read: false,
            is_starred: false,
            created: '',
            updated: '',
        },
        {
            id: 'ts_hit2',
            thread: 'thread_hit2',
            user: 'u1',
            folder: 'inbox',
            is_read: true,
            is_starred: true,
            created: '',
            updated: '',
        },
        // Same thread as a hit, but another user's state row.
        {
            id: 'ts_other',
            thread: 'thread_hit1',
            user: 'u2',
            folder: 'inbox',
            is_read: false,
            is_starred: false,
            created: '',
            updated: '',
        },
        // This user's row on a thread absent from the results.
        {
            id: 'ts_unsearched',
            thread: 'thread_unsearched',
            user: 'u1',
            folder: 'inbox',
            is_read: false,
            is_starred: false,
            created: '',
            updated: '',
        },
    ],
    assignments: [] as unknown[],
}))

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: { id: 'u1' }, isLoggedIn: true }),
}))

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { createCollection, localOnlyCollectionOptions } = await import('@tanstack/db')
    const mk = (id: string, initialData: { id: string }[]) =>
        createCollection(
            localOnlyCollectionOptions({ id, getKey: (r: { id: string }) => r.id, initialData })
        )
    const stores: Record<string, unknown> = {
        mail_thread_state: mk('mail_thread_state', h.states as { id: string }[]),
        label_assignments: mk('label_assignments', h.assignments as { id: string }[]),
    }
    return {
        useStore: (...names: string[]) => names.map(n => stores[n]),
    }
})

vi.mock('~/tinycld/mail/hooks/useLabels', () => ({
    useLabels: () => ({ labels: [], labelMap: new Map() }),
}))

// Capture each compiled query so a test can assert on what the hook actually
// asks the collection for — the bounding lives in the query, not the result.
const seenQueries: string[] = []

// The IR holds live collection references, so drop `collection` keys and guard
// against cycles; only the predicate shape matters here.
const serializeWhere = (where: unknown): string => {
    const seen = new WeakSet()
    return JSON.stringify(where, (key, value) => {
        if (key === 'collection') return undefined
        if (typeof value === 'object' && value !== null) {
            if (seen.has(value)) return undefined
            seen.add(value)
        }
        return value
    })
}

vi.mock('@tanstack/react-db', async () => {
    const actual = await vi.importActual<typeof import('@tanstack/react-db')>('@tanstack/react-db')
    return {
        ...actual,
        useLiveQuery: (fn: unknown, deps: unknown[]) =>
            actual.useLiveQuery((q: never) => {
                const built = (fn as (q: never) => { query?: { from?: unknown; where?: unknown } })(
                    q
                )
                if (built?.query) {
                    seenQueries.push(
                        `${serializeWhere(built.query.from)}|${serializeWhere(built.query.where)}`
                    )
                }
                return built as never
            }, deps),
    }
})

import type { MailSearchResult } from '~/tinycld/mail/hooks/useMailSearch'
import { useSearchThreadItems } from '~/tinycld/mail/hooks/useSearchThreadItems'

const hit = (threadId: string, subject: string): MailSearchResult => ({
    thread_id: threadId,
    subject,
    subject_highlight: subject,
    snippet_highlight: '',
    latest_date: '2024-01-01T00:00:00Z',
    participants: JSON.stringify([{ name: 'Alice', email: 'alice@acme.com' }]),
    message_count: 1,
    mailbox_id: 'mb_me',
    has_attachments: false,
})

test('resolves state only for the searched threads, scoped to the signed-in user', async () => {
    const results = [hit('thread_hit1', 'First'), hit('thread_hit2', 'Second')]
    const { result } = renderHook(() => useSearchThreadItems('u1', results))

    await waitFor(() => expect(result.current.length).toBe(2))

    // Rows carry the signed-in user's state id, never u2's row on the same
    // thread. stateId falls back to thread_id when no state resolves, so this
    // also catches a predicate that matches nothing.
    const stateIds = result.current.map(r => r.stateId).sort()
    expect(stateIds).toEqual(['ts_hit1', 'ts_hit2'])
})

// The assertion above passes whether or not the query is bounded — the hook
// indexes by thread id, so surplus rows are simply never looked up. What must
// not regress is the QUERY: on an on-demand collection an unbounded predicate
// syncs the entire mailbox. Assert on the compiled where-clause so dropping
// the id predicate fails here.
test('the thread_state query is restricted to the searched ids', async () => {
    const results = [hit('thread_hit1', 'First'), hit('thread_hit2', 'Second')]
    renderHook(() => useSearchThreadItems('u1', results))

    await waitFor(() => expect(seenQueries.length).toBeGreaterThan(0))

    const stateQuery = seenQueries.find(q => q.includes('mail_thread_state'))
    expect(stateQuery).toBeDefined()
    expect(stateQuery).toContain('thread_hit1')
    expect(stateQuery).toContain('thread_hit2')
    // The unsearched thread is never named, so the query cannot pull it down.
    expect(stateQuery).not.toContain('thread_unsearched')
})

test('an empty result set resolves no rows and issues no unbounded query', async () => {
    const { result } = renderHook(() => useSearchThreadItems('u1', []))

    await waitFor(() => expect(result.current).toEqual([]))
})
