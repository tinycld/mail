import { eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { useMemo } from 'react'
import type { FolderCounts } from './computeMailboxFolderCounts'
import { computeMailboxFolderCounts } from './computeMailboxFolderCounts'

export type { FolderCounts } from './computeMailboxFolderCounts'
export { computeMailboxFolderCounts }

const EMPTY: FolderCounts = { inbox: 0, drafts: 0, sent: 0, starred: 0, trash: 0, spam: 0 }

/**
 * Reads per-mailbox folder counts from the mail_folder_counts view collection.
 *
 * The view aggregates mail_thread_state × mail_threads server-side, so the
 * sidebar no longer needs to fetch every state row + every thread to count
 * client-side. PocketBase pushes view-row updates whenever the underlying
 * tables change, so the counts stay live.
 */
export function useMailboxFolderCounts(): Map<string, FolderCounts> {
    const [countsCollection] = useStore('mail_folder_counts')

    const { data: rows } = useOrgLiveQuery((query, { userOrgId }) =>
        query
            .from({ counts: countsCollection })
            .where(({ counts }) => eq(counts.user_org, userOrgId))
    )

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

// Re-export the empty constant for callers that need a default — keeps the
// public API stable for places that historically read counts via the pure
// helper.
export { EMPTY as EMPTY_FOLDER_COUNTS }
