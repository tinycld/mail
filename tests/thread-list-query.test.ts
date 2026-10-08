import { and, IR, type Ref } from '@tanstack/db'
import { convertToPocketBaseFilter } from 'pbtsdb/core'
import { describe, expect, it } from 'vitest'
import {
    countsKeyForFolder,
    cursorTerms,
    decodeCursors,
    encodeCursors,
    folderTerms,
    hasNextPage,
} from '~/tinycld/mail/lib/thread-list-query'
import type { MailThreadState } from '~/tinycld/mail/types'

// A ref proxy stands in for the row: TanStack's where callbacks receive one,
// and every field read is a property reference. The converter is pbtsdb's
// own, so these assertions pin the PocketBase filter the server receives.
const s = new Proxy({} as Record<string, unknown>, {
    get: (_target, name) => new IR.PropRef([String(name)]),
}) as unknown as Ref<MailThreadState>

const compile = (terms: ReturnType<typeof folderTerms>) =>
    convertToPocketBaseFilter(terms.length === 1 ? terms[0] : and(...terms))

describe('folderTerms', () => {
    it('inbox and null both filter folder = inbox', () => {
        expect(compile(folderTerms(s, 'inbox'))).toBe('folder = "inbox"')
        expect(compile(folderTerms(s, null))).toBe('folder = "inbox"')
    })
    it('starred reads the flag, any folder', () => {
        expect(compile(folderTerms(s, 'starred'))).toBe('is_starred = true')
    })
    it('sent reads the flag and excludes trash and spam', () => {
        expect(compile(folderTerms(s, 'sent'))).toBe(
            '(is_sent = true && (folder != "trash" && folder != "spam"))'
        )
    })
    it('all adds no term', () => {
        expect(folderTerms(s, 'all')).toEqual([])
    })
    it('all-inboxes filters to the inbox folder', () => {
        expect(compile(folderTerms(s, 'all-inboxes'))).toBe('folder = "inbox"')
    })
    it('other folders filter by name', () => {
        expect(compile(folderTerms(s, 'archive'))).toBe('folder = "archive"')
    })
})

describe('cursorTerms', () => {
    it('no cursor adds no term', () => {
        expect(cursorTerms(s, null)).toEqual([])
    })
    it('a cursor selects rows strictly after it in (latest_date desc, id desc) order', () => {
        const filter = compile(cursorTerms(s, { date: '2026-10-05 10:00:00.000Z', id: 'abc' }))
        expect(filter).toBe(
            '(latest_date < "2026-10-05 10:00:00.000Z" || (latest_date = "2026-10-05 10:00:00.000Z" && id < "abc"))'
        )
    })
})

describe('cursor codec', () => {
    it('round-trips a stack', () => {
        const stack = [
            { date: '2026-10-05 10:00:00.000Z', id: 'a1' },
            { date: '2026-10-04 09:00:00.000Z', id: 'b2' },
        ]
        expect(decodeCursors(encodeCursors(stack))).toEqual(stack)
    })
    it('decodes nothing from undefined or garbage', () => {
        expect(decodeCursors(undefined)).toEqual([])
        expect(decodeCursors('')).toEqual([])
        expect(decodeCursors('no-separator')).toEqual([])
    })
})

describe('countsKeyForFolder', () => {
    it('maps every folder to its total column', () => {
        expect(countsKeyForFolder(null)).toBe('inboxTotal')
        expect(countsKeyForFolder('inbox')).toBe('inboxTotal')
        expect(countsKeyForFolder('all-inboxes')).toBe('inboxTotal')
        expect(countsKeyForFolder('all')).toBe('total')
        expect(countsKeyForFolder('sent')).toBe('sent')
        expect(countsKeyForFolder('archive')).toBe('archive')
    })
})

describe('hasNextPage', () => {
    const cursor = { date: '2026-10-05 10:00:00.000Z', id: 'abc' }
    const base = { hasFullPage: true, nextCursor: cursor, page: 1, pageSize: 100 }
    it('a full page with more rows behind it has a next page', () => {
        expect(hasNextPage({ ...base, totalItems: 101 })).toBe(true)
    })
    it('a folder of exactly one full page has no next page', () => {
        expect(hasNextPage({ ...base, totalItems: 100 })).toBe(false)
    })
    it('the last full page of a deeper folder has no next page', () => {
        expect(hasNextPage({ ...base, page: 2, totalItems: 200 })).toBe(false)
    })
    it('a short page has no next page whatever the total says', () => {
        expect(hasNextPage({ ...base, hasFullPage: false, totalItems: 500 })).toBe(false)
    })
    it('no cursor means no next page', () => {
        expect(hasNextPage({ ...base, nextCursor: null, totalItems: 500 })).toBe(false)
    })
})
