import { and, eq, type IR, inArray, lt, not, or, type Ref } from '@tanstack/db'
import type { FolderCounts } from '../hooks/useMailboxFolderCounts'
import type { MailThreadState } from '../types'

export interface ThreadCursor {
    date: string
    id: string
}

type StateRef = Ref<MailThreadState>
// @tanstack/db 0.9.0 doesn't export BasicExpression as a top-level named
// type — only via the IR namespace export.
type Term = IR.BasicExpression<boolean>

// Folder semantics shared with the server's folder counts (folder_counts.go):
//   inbox    — folder = 'inbox'
//   all-inboxes — folder = 'inbox'; the mailbox scope is widened by the caller
//   starred  — is_starred, any folder
//   sent     — is_sent, outside trash and spam (a reply flags its thread sent
//              and leaves it where it was)
//   all      — every row for the user in scope
//   <other>  — folder = <name>
export function folderTerms(s: StateRef, folder: string | null): Term[] {
    const key = folder ?? 'inbox'
    if (key === 'starred') return [eq(s.is_starred, true)]
    if (key === 'sent') return [and(eq(s.is_sent, true), not(inArray(s.folder, ['trash', 'spam'])))]
    if (key === 'all') return []
    if (key === 'all-inboxes') return [eq(s.folder, 'inbox')]
    return [eq(s.folder, key)]
}

// The page boundary lives in the where, so a deep page loads only its rows
// and subscribes only to its slice. Rows sort by (latest_date desc, id desc);
// the id tiebreaker keeps the boundary exact when two threads share a date.
export function cursorTerms(s: StateRef, cursor: ThreadCursor | null): Term[] {
    if (!cursor) return []
    return [
        or(
            lt(s.latest_date, cursor.date),
            and(eq(s.latest_date, cursor.date), lt(s.id, cursor.id))
        ),
    ]
}

// A full page alone cannot tell "more rows" from "exactly pageSize rows", so
// the live folder total decides; a lagging counts row only disables Older
// until it catches up.
export function hasNextPage(args: {
    hasFullPage: boolean
    nextCursor: ThreadCursor | null
    page: number
    pageSize: number
    totalItems: number
}): boolean {
    const { hasFullPage, nextCursor, page, pageSize, totalItems } = args
    return hasFullPage && nextCursor !== null && page * pageSize < totalItems
}

// URL form: one cursor per previous page, `date|id`, comma-joined. Dates and
// ids contain neither character.
const FIELD_SEPARATOR = '|'
const CURSOR_SEPARATOR = ','

export function encodeCursors(cursors: ThreadCursor[]): string {
    return cursors.map(c => `${c.date}${FIELD_SEPARATOR}${c.id}`).join(CURSOR_SEPARATOR)
}

export function decodeCursors(raw: string | undefined): ThreadCursor[] {
    if (!raw) return []
    const out: ThreadCursor[] = []
    for (const part of raw.split(CURSOR_SEPARATOR)) {
        const at = part.indexOf(FIELD_SEPARATOR)
        if (at <= 0 || at === part.length - 1) return []
        out.push({ date: part.slice(0, at), id: part.slice(at + 1) })
    }
    return out
}

// Which counts column is the page indicator's total for a folder view.
export function countsKeyForFolder(folder: string | null): keyof FolderCounts {
    switch (folder ?? 'inbox') {
        case 'inbox':
        case 'all-inboxes':
            return 'inboxTotal'
        case 'all':
            return 'total'
        case 'starred':
            return 'starred'
        case 'sent':
            return 'sent'
        case 'drafts':
            return 'drafts'
        case 'trash':
            return 'trash'
        case 'spam':
            return 'spam'
        default:
            return 'archive'
    }
}
