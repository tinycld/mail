// @vitest-environment happy-dom
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

// No vitest globals in this workspace, so testing-library's automatic
// afterEach cleanup never registers — without this, a previous test's mounted
// hook keeps its thread_state subscription alive and pollutes the spy.
afterEach(cleanup)

// The sidebar's counts come from the mail_folder_counts VIEW, not from a
// client-side aggregation. These tests mount the real hook against real
// TanStack DB collections so the query predicate actually executes — and they
// read the column names from the SHIPPED migration rather than re-declaring
// them, so a rename in the view goes red here instead of silently returning
// zero rows (the §3.2 bug class).

const h = vi.hoisted(() => {
    const viewRows = [
        {
            id: 'u1:mb1',
            user: 'u1',
            mailbox: 'mb1',
            inbox: 3,
            drafts: 1,
            sent: 2,
            starred: 4,
            trash: 0,
            spam: 1,
        },
        {
            id: 'u1:mb2',
            user: 'u1',
            mailbox: 'mb2',
            inbox: 0,
            drafts: 0,
            sent: 5,
            starred: 0,
            trash: 2,
            spam: 0,
        },
        // Another user's row: must be filtered out by the hook's user predicate.
        {
            id: 'u2:mb3',
            user: 'u2',
            mailbox: 'mb3',
            inbox: 9,
            drafts: 9,
            sent: 9,
            starred: 9,
            trash: 9,
            spam: 9,
        },
    ]
    return { viewRows, invalidateQueries: vi.fn() }
})

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: { id: 'u1' }, isLoggedIn: true }),
}))

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { createCollection, localOnlyCollectionOptions } = await import('@tanstack/db')
    const counts = createCollection(
        localOnlyCollectionOptions({
            id: 'mail_folder_counts',
            getKey: (r: { id: string }) => r.id,
            initialData: h.viewRows,
        })
    )
    const threadState = createCollection(
        localOnlyCollectionOptions({
            id: 'mail_thread_state',
            getKey: (r: { id: string }) => r.id,
        })
    )
    const stores: Record<string, unknown> = {
        mail_folder_counts: counts,
        mail_thread_state: threadState,
    }
    return {
        useStore: (...names: string[]) => names.map(n => stores[n]),
        queryClient: { invalidateQueries: h.invalidateQueries },
        __test: { counts, threadState },
    }
})

import * as pbModule from '@tinycld/core/lib/pocketbase'
import { useMailboxFolderCounts } from '~/tinycld/mail/hooks/useMailboxFolderCounts'

const { threadState } = (
    pbModule as unknown as {
        __test: { threadState: { insert: (row: Record<string, unknown>) => void } }
    }
).__test

test('the shipped view declares exactly the columns the hook and these fixtures read', () => {
    const src = readFileSync(
        join(
            import.meta.dirname,
            '..',
            'pb-migrations',
            '1830000000_create_mail_folder_counts_view.js'
        ),
        'utf8'
    )
    const aliases = [...src.matchAll(/\bAS\s+(\w+)/g)].map(m => m[1])
    // The fixture rows above must be keyed by the shipped columns — if the view
    // renames or drops one, this fails loudly instead of the hook silently
    // reading undefined.
    expect(aliases.sort()).toEqual(Object.keys(h.viewRows[0]).sort())
    // The realtime bridge below subscribes to mail_thread_state because that is
    // the table the view aggregates; if the view's source moves, so must the
    // subscription in useMailboxFolderCounts.
    expect(src).toContain('FROM mail_thread_state')
    expect(src).toContain('JOIN mail_threads')
})

test('returns only the signed-in user’s rows, keyed by mailbox with per-folder counts', async () => {
    const { result } = renderHook(() => useMailboxFolderCounts())

    await waitFor(() => expect(result.current.size).toBeGreaterThan(0))

    expect(result.current.get('mb1')).toEqual({
        inbox: 3,
        drafts: 1,
        sent: 2,
        starred: 4,
        trash: 0,
        spam: 1,
    })
    expect(result.current.get('mb2')).toEqual({
        inbox: 0,
        drafts: 0,
        sent: 5,
        starred: 0,
        trash: 2,
        spam: 0,
    })
    // u2's row must be excluded by the eq(counts.user, userId) predicate.
    expect(result.current.has('mb3')).toBe(false)
    expect(result.current.size).toBe(2)
})

test('a local mail_thread_state change invalidates the counts query (view realtime bridge)', async () => {
    h.invalidateQueries.mockClear()
    const { unmount } = renderHook(() => useMailboxFolderCounts())

    threadState.insert({
        id: 's1',
        thread: 't1',
        user: 'u1',
        folder: 'inbox',
        is_read: false,
        is_starred: false,
        created: '',
        updated: '',
    })

    await waitFor(() =>
        expect(h.invalidateQueries).toHaveBeenCalledWith({
            queryKey: ['mail_folder_counts'],
        })
    )

    // After unmount the subscription must be released — further changes no
    // longer invalidate.
    h.invalidateQueries.mockClear()
    unmount()
    threadState.insert({
        id: 's2',
        thread: 't2',
        user: 'u1',
        folder: 'inbox',
        is_read: false,
        is_starred: false,
        created: '',
        updated: '',
    })
    await new Promise(resolve => setTimeout(resolve, 50))
    expect(h.invalidateQueries).not.toHaveBeenCalled()
})
