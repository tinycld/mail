import { eq } from '@tanstack/db'
import { queryClient, useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { useEffect, useMemo } from 'react'

export interface FolderCounts {
    inbox: number
    drafts: number
    sent: number
    starred: number
    trash: number
    spam: number
}

/**
 * Reads per-mailbox folder counts from the mail_folder_counts view collection.
 *
 * The view aggregates mail_thread_state × mail_threads server-side. PocketBase
 * does NOT emit realtime events for view collections, so we bridge the gap by
 * subscribing to local mail_thread_state changes (which fire on optimistic
 * writes and incoming realtime events) and invalidating the counts query so
 * it refetches.
 */
export function useMailboxFolderCounts(): Map<string, FolderCounts> {
    const [countsCollection, threadStateCollection] = useStore(
        'mail_folder_counts',
        'mail_thread_state'
    )

    const { data: rows } = useOrgLiveQuery((query, { userId }) =>
        query.from({ counts: countsCollection }).where(({ counts }) => eq(counts.user, userId))
    )

    useEffect(() => {
        const sub = threadStateCollection.subscribeChanges(() => {
            queryClient.invalidateQueries({ queryKey: ['mail_folder_counts'] })
        })
        return () => sub.unsubscribe()
    }, [threadStateCollection])

    return useMemo(() => {
        const map = new Map<string, FolderCounts>()
        for (const r of rows ?? []) {
            map.set(r.mailbox, {
                inbox: r.inbox ?? 0,
                drafts: r.drafts ?? 0,
                sent: r.sent ?? 0,
                starred: r.starred ?? 0,
                trash: r.trash ?? 0,
                spam: r.spam ?? 0,
            })
        }
        return map
    }, [rows])
}
