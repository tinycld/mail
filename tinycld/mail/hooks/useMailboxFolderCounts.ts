import { eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { useMemo } from 'react'
import { countsKeyForFolder } from '../lib/thread-list-query'

export interface FolderCounts {
    /** Unread threads in the inbox: the sidebar badge. */
    inbox: number
    /** Every thread in the inbox: the page indicator's total. */
    inboxTotal: number
    archive: number
    drafts: number
    sent: number
    starred: number
    trash: number
    spam: number
    /** All Mail. */
    total: number
}

export const EMPTY_COUNTS: FolderCounts = {
    inbox: 0,
    inboxTotal: 0,
    archive: 0,
    drafts: 0,
    sent: 0,
    starred: 0,
    trash: 0,
    spam: 0,
    total: 0,
}

/**
 * Per-mailbox folder counts for the signed-in user. mail_folder_counts is a
 * base collection a Go hook recomputes on every mail_thread_state change
 * (server/folder_counts.go), so realtime keeps this query current with no
 * bridge.
 */
export function useMailboxFolderCounts(): Map<string, FolderCounts> {
    const [countsCollection] = useStore('mail_folder_counts')

    const { data: rows } = useMyLiveQuery((query, { userId }) =>
        query.from({ counts: countsCollection }).where(({ counts }) => eq(counts.user, userId))
    )

    return useMemo(() => {
        const map = new Map<string, FolderCounts>()
        for (const r of rows ?? []) {
            map.set(r.mailbox, {
                inbox: r.inbox,
                inboxTotal: r.inbox_total,
                archive: r.archive,
                drafts: r.drafts,
                sent: r.sent,
                starred: r.starred,
                trash: r.trash,
                spam: r.spam,
                total: r.total,
            })
        }
        return map
    }, [rows])
}

/** The folder's thread total across the given mailboxes. */
export function folderTotal(
    counts: Map<string, FolderCounts>,
    mailboxIds: string[],
    folder: string | null
): number {
    const key = countsKeyForFolder(folder)
    let sum = 0
    for (const id of mailboxIds) sum += counts.get(id)?.[key] ?? 0
    return sum
}
