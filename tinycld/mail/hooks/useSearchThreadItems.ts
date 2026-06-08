import { and, eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { useMemo } from 'react'
import type { ThreadListItem } from '../components/thread-list-item'
import type { MailThreadState } from '../types'
import { searchResultToThreadListItem } from './mailListHelpers'
import { useLabels } from './useLabels'
import type { MailSearchResult } from './useMailSearch'

/**
 * Turns raw FTS search hits into list rows backed by real mail_thread_state.
 *
 * Search hits come from a server FTS endpoint that returns thread display data
 * but no thread_state id — so on their own a hit's swipe actions (archive /
 * trash / star) would target `thread_id` as if it were a state id and silently
 * no-op. mail_thread_state is eager and bounded per user, so we live-query it
 * here, index it by thread id, and merge each hit with its resolved state. Hits
 * with no resolvable state (shouldn't happen — a searchable thread the user can
 * see has a state row) are dropped so the list never shows an un-actionable row.
 *
 * Because the merge reads the live thread_state, search rows reflect the real
 * read / starred / folder / label state and update in place as those change.
 */
export function useSearchThreadItems(
    userOrgId: string,
    results: MailSearchResult[]
): ThreadListItem[] {
    const [threadStateCollection, assignmentsCollection] = useStore(
        'mail_thread_state',
        'label_assignments'
    )
    const { labelMap } = useLabels()

    const { data: threadStates } = useOrgLiveQuery(
        query =>
            query
                .from({ mail_thread_state: threadStateCollection })
                .where(({ mail_thread_state }) => eq(mail_thread_state.user_org, userOrgId)),
        [userOrgId]
    )

    const { data: allAssignments } = useOrgLiveQuery((query, { userOrgId }) =>
        query
            .from({ label_assignments: assignmentsCollection })
            .where(({ label_assignments }) =>
                and(
                    eq(label_assignments.collection, 'mail_thread_state'),
                    eq(label_assignments.user_org, userOrgId)
                )
            )
    )

    const stateByThread = useMemo(() => {
        const map = new Map<string, MailThreadState>()
        for (const s of (threadStates ?? []) as MailThreadState[]) map.set(s.thread, s)
        return map
    }, [threadStates])

    const labelIdsByRecord = useMemo(() => {
        const map = new Map<string, string[]>()
        for (const a of allAssignments ?? []) {
            const list = map.get(a.record_id) ?? []
            list.push(a.label)
            map.set(a.record_id, list)
        }
        return map
    }, [allAssignments])

    return useMemo(() => {
        const out: ThreadListItem[] = []
        for (const result of results) {
            const state = stateByThread.get(result.thread_id)
            if (!state) continue
            const labels = (labelIdsByRecord.get(state.id) ?? [])
                .map(id => labelMap.get(id))
                .filter((l): l is { id: string; name: string; color: string } => l != null)
            out.push(searchResultToThreadListItem(result, state, labels))
        }
        return out
    }, [results, stateByThread, labelIdsByRecord, labelMap])
}
