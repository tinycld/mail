import { and, type Ref } from '@tanstack/db'
import { convertToPocketBaseFilter } from 'pbtsdb/core'
import { describe, expect, it } from 'vitest'
import {
    countsKeyForFolder,
    cursorTerms,
    decodeCursors,
    encodeCursors,
    folderTerms,
} from '~/tinycld/mail/lib/thread-list-query'
import type { MailThreadState } from '~/tinycld/mail/types'

// A ref proxy stands in for the row: TanStack's where callbacks receive one,
// and the converter only reads field paths. The converter is pbtsdb's own,
// so these assertions pin the PocketBase filter the server receives.
const s = new Proxy({} as Record<string, unknown>, {
    get: (_target, name) => ({ type: 'ref', path: [String(name)] }),
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
    it('all and all-inboxes add no term', () => {
        expect(folderTerms(s, 'all')).toEqual([])
        expect(folderTerms(s, 'all-inboxes')).toEqual([])
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
