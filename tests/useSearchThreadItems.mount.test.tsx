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

const h = vi.hoisted(() => ({
    states: [
        {
            id: 'ts_hit1',
            thread: 'thread_hit1',
            user: 'u1',
            mailbox: 'mb1',
            folder: 'inbox',
            is_read: false,
            is_starred: false,
            is_sent: false,
            latest_date: '2026-10-05 10:00:00.000Z',
            created: '',
            updated: '',
        },
        {
            id: 'ts_hit2',
            thread: 'thread_hit2',
            user: 'u1',
            mailbox: 'mb1',
            folder: 'inbox',
            is_read: true,
            is_starred: true,
            is_sent: false,
            latest_date: '2026-10-04 10:00:00.000Z',
            created: '',
            updated: '',
        },
        // This user's row on a thread absent from the results.
        {
            id: 'ts_unsearched',
            thread: 'thread_unsearched',
            user: 'u1',
            mailbox: 'mb1',
            folder: 'inbox',
            is_read: false,
            is_starred: false,
            is_sent: false,
            latest_date: '2026-10-03 10:00:00.000Z',
            created: '',
            updated: '',
        },
    ],
    threads: [
        {
            id: 'thread_hit1',
            mailbox: 'mb1',
            subject: 'Hit one',
            snippet: 'stored snippet one',
            message_count: 2,
            latest_date: '2026-10-05 10:00:00.000Z',
            participants: [{ name: 'Ann', email: 'ann@example.com' }],
            has_draft: false,
            has_attachments: true,
            created: '',
            updated: '',
        },
        {
            id: 'thread_hit2',
            mailbox: 'mb1',
            subject: 'Hit two',
            snippet: 'stored snippet two',
            message_count: 1,
            latest_date: '2026-10-04 10:00:00.000Z',
            participants: [],
            has_draft: false,
            has_attachments: false,
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
    const { BasicIndex, createCollection, localOnlyCollectionOptions } = await import(
        '@tanstack/db'
    )
    const mk = (id: string, initialData: { id: string }[]) => {
        const collection = createCollection({
            ...localOnlyCollectionOptions({ id, getKey: (r: { id: string }) => r.id, initialData }),
            autoIndex: 'eager',
            defaultIndexType: BasicIndex,
        })
        // The real collection is a pbtsdb collection; fetchRelations returns a
        // view sharing its store. Here the base collection stands in for the view.
        return Object.assign(collection, { fetchRelations: () => collection })
    }
    const stores: Record<string, unknown> = {
        mail_thread_state: mk('mail_thread_state', h.states as { id: string }[]),
        mail_threads: mk('mail_threads', h.threads as { id: string }[]),
        label_assignments: mk('label_assignments', h.assignments as { id: string }[]),
    }
    return { useStore: (...names: string[]) => names.map(n => stores[n]) }
})

vi.mock('~/tinycld/mail/hooks/useLabels', () => ({
    useLabels: () => ({ labels: [], labelMap: new Map() }),
}))

// Capture each compiled query so a test can assert on what the hook actually
// asks the collection for — the bounding lives in the query, not the result.
// Each entry is `form|from|where`, so a test locates the query it cares about
// by its FROM segment (the collection's identity) before reading its WHERE
// segment — the hook's query aliases the collection as `s`, carrying no
// collection name, so `from` is serialized separately to keep the real
// collection's `id` (otherwise invisible once the cyclic `collection` object
// is stripped) while `where` stays alias-shaped.
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

// Unlike `serializeWhere`, this keeps the collection's `id` (dropping the rest
// of the live collection object, which is cyclic) so a test can identify which
// real collection a query's FROM clause names, even though the query aliases
// it as something else (`s`).
const serializeFrom = (from: unknown): string =>
    JSON.stringify(from, (key, value) => {
        if (key === 'collection' && value && typeof value === 'object' && 'id' in value) {
            return { id: (value as { id: unknown }).id }
        }
        return value
    })

// Each entry records the call form too: the hook must use the object form
// (`{ query }`), while core's useMyLiveQuery may still pass `(fn, deps)`.
type QueryFn = (q: never) => { query?: { from?: unknown; where?: unknown } } | null | undefined

const recordQuery = (fn: QueryFn, form: 'object' | 'deps') => (q: never) => {
    const built = fn(q)
    if (built?.query) {
        seenQueries.push(
            `${form}|${serializeFrom(built.query.from)}|${serializeWhere(built.query.where)}`
        )
    }
    return built as never
}

vi.mock('@tanstack/react-db', async () => {
    const actual = await vi.importActual<typeof import('@tanstack/react-db')>('@tanstack/react-db')
    return {
        ...actual,
        useLiveQuery: (arg: QueryFn | { query: QueryFn }, deps?: unknown[]) => {
            const config =
                typeof arg === 'function'
                    ? recordQuery(arg, deps ? 'deps' : 'object')
                    : { ...arg, query: recordQuery(arg.query, 'object') }
            return actual.useLiveQuery(config as never, deps as never)
        },
    }
})

import { useSearchThreadItems } from '~/tinycld/mail/hooks/useSearchThreadItems'

const results = [
    {
        thread_id: 'thread_hit2',
        state_id: 'ts_hit2',
        subject_highlight: '<mark>Hit</mark> two',
        snippet_highlight: 'two <mark>match</mark>',
    },
    { thread_id: 'thread_hit1', state_id: 'ts_hit1', subject_highlight: '', snippet_highlight: '' },
]

test('rows come from the live state and thread, in hit order, with the highlight snippet', async () => {
    const { result } = renderHook(() => useSearchThreadItems(results))
    await waitFor(() => expect(result.current).toHaveLength(2))
    expect(result.current.map(i => i.stateId)).toEqual(['ts_hit2', 'ts_hit1'])
    expect(result.current[0]).toMatchObject({
        threadId: 'thread_hit2',
        subject: 'Hit two',
        snippet: 'two match',
        isRead: true,
        isStarred: true,
        folder: 'inbox',
    })
    expect(result.current[1]).toMatchObject({
        subject: 'Hit one',
        snippet: 'stored snippet one',
        senderName: 'Ann',
        hasAttachments: true,
    })
})

// The hook's query aliases the state collection as `s`, so a query is "the
// state query" by what its FROM names, not by any string in the whole entry —
// the label-assignments query legitimately carries 'mail_thread_state' too,
// as the VALUE of its `collection` filter, so matching on the whole entry
// would find the wrong query.
const readsThreadState = (q: string) => q.split('|')[1]?.includes('mail_thread_state')

test('the state query is bounded to the hit ids and carries no user term', () => {
    renderHook(() => useSearchThreadItems(results))
    const stateQuery = seenQueries.find(readsThreadState)
    expect(stateQuery).toBeDefined()
    const whereSegment = stateQuery?.split('|')[2]
    expect(whereSegment).toContain('ts_hit1')
    expect(whereSegment).toContain('ts_hit2')
    expect(whereSegment).not.toContain('"user"')
})

test('no results, no state query', () => {
    const { result } = renderHook(() => useSearchThreadItems([]))
    expect(result.current).toEqual([])
    expect(seenQueries.some(readsThreadState)).toBe(false)
})
