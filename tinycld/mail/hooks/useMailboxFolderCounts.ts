import { eq } from '@tanstack/db'
import { queryClient, useStore } from '@tinycld/core/lib/pocketbase'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
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
 * does NOT emit realtime events for view collections — on-demand + per-query
 * realtime (pbtsdb 0.10) doesn't change that, the filtered subscription topic
 * is just as inert as the old whole-collection one was. So we bridge the gap
 * by subscribing to local mail_thread_state changes (which fire on optimistic
 * writes and incoming realtime events) and invalidating the counts query so
 * it refetches. The bare `['mail_folder_counts']` key still prefix-matches
 * pbtsdb's on-demand cache keys (`[name, request]`).
 */
export function useMailboxFolderCounts(): Map<string, FolderCounts> {
    const [countsCollection, threadStateCollection] = useStore(
        'mail_folder_counts',
        'mail_thread_state'
    )

    const { data: rows } = useMyLiveQuery((query, { userId }) =>
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
