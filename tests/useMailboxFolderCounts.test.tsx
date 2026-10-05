// @vitest-environment happy-dom
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(cleanup)

// mail_folder_counts is a base collection maintained by Go hooks
// (server/folder_counts.go), so realtime covers it and the hook is a plain
// live query. These tests mount the real hook against a real TanStack DB
// collection so the user predicate executes, and read the column names from
// the shipped migration so a rename there goes red here.

const h = vi.hoisted(() => ({
    rows: [
        {
            id: 'u1mb1',
            user: 'u1',
            mailbox: 'mb1',
            inbox: 3,
            inbox_total: 10,
            archive: 4,
            drafts: 1,
            sent: 2,
            starred: 4,
            trash: 0,
            spam: 1,
            total: 20,
        },
        {
            id: 'u1mb2',
            user: 'u1',
            mailbox: 'mb2',
            inbox: 0,
            inbox_total: 5,
            archive: 0,
            drafts: 0,
            sent: 5,
            starred: 0,
            trash: 2,
            spam: 0,
            total: 7,
        },
        {
            id: 'u2mb3',
            user: 'u2',
            mailbox: 'mb3',
            inbox: 9,
            inbox_total: 9,
            archive: 9,
            drafts: 9,
            sent: 9,
            starred: 9,
            trash: 9,
            spam: 9,
            total: 9,
        },
    ],
}))

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: { id: 'u1' }, isLoggedIn: true }),
}))

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { createCollection, localOnlyCollectionOptions } = await import('@tanstack/db')
    const counts = createCollection(
        localOnlyCollectionOptions({
            id: 'mail_folder_counts',
            getKey: (r: { id: string }) => r.id,
            initialData: h.rows,
        })
    )
    return { useStore: () => [counts] }
})

import {
    EMPTY_COUNTS,
    folderTotal,
    useMailboxFolderCounts,
} from '~/tinycld/mail/hooks/useMailboxFolderCounts'

test('the shipped migration declares exactly the number columns the hook reads', () => {
    const src = readFileSync(
        join(import.meta.dirname, '..', 'pb-migrations', '1830000013_folder_counts_table.js'),
        'utf8'
    )
    const upSection = src.slice(0, src.indexOf('app => {', src.indexOf('INSERT INTO')))
    const columns = [...upSection.matchAll(/number\('mail_fc_\w+', '(\w+)'\)/g)].map(m => m[1])
    const fixtureNumbers = Object.keys(h.rows[0]).filter(
        k => !['id', 'user', 'mailbox'].includes(k)
    )
    expect(columns.sort()).toEqual(fixtureNumbers.sort())
})

test('returns only the signed-in user’s rows, keyed by mailbox', async () => {
    const { result } = renderHook(() => useMailboxFolderCounts())
    await waitFor(() => expect(result.current.size).toBe(2))
    expect(result.current.get('mb1')).toEqual({
        inbox: 3,
        inboxTotal: 10,
        archive: 4,
        drafts: 1,
        sent: 2,
        starred: 4,
        trash: 0,
        spam: 1,
        total: 20,
    })
    expect(result.current.has('mb3')).toBe(false)
})

test('folderTotal sums the folder column across the given mailboxes', async () => {
    const { result } = renderHook(() => useMailboxFolderCounts())
    await waitFor(() => expect(result.current.size).toBe(2))
    expect(folderTotal(result.current, ['mb1'], 'inbox')).toBe(10)
    expect(folderTotal(result.current, ['mb1', 'mb2'], 'all-inboxes')).toBe(15)
    expect(folderTotal(result.current, ['mb1', 'mb2'], 'all')).toBe(27)
    expect(folderTotal(result.current, ['mb2'], 'trash')).toBe(2)
    expect(folderTotal(result.current, ['missing'], 'inbox')).toBe(0)
    expect(EMPTY_COUNTS.total).toBe(0)
})
