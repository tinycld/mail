import { and, eq, inArray } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { materialize } from 'pbtsdb'
import { useMemo } from 'react'
import {
    type ThreadListItem,
    type ThreadListRow,
    toThreadListItem,
} from '../components/thread-list-item'
import { stripHtmlTags } from './mailListHelpers'
import { useLabels } from './useLabels'
import type { MailSearchResult } from './useMailSearch'

type LabelInfo = { id: string; name: string; color: string }
const isLabel = (l: LabelInfo | undefined): l is LabelInfo => l != null

/**
 * Turns FTS hits into list rows. A hit carries only its state id, thread id,
 * and highlights; the row renders from the live mail_thread_state and
 * mail_threads rows, so archive, star, and read changes show in place and a
 * new message in a hit updates its snippet.
 *
 * The state query is a pure id subset: served from the store when the rows
 * are present, otherwise one batched request. No user term: the state rule is
 * `user = me` and the endpoint scoped the ids to the caller.
 */
export function useSearchThreadItems(results: MailSearchResult[]): ThreadListItem[] {
    const [threadStateCollection, threadsCollection, assignmentsCollection] = useStore(
        'mail_thread_state',
        'mail_threads',
        'label_assignments'
    )
    const { labelMap } = useLabels()

    const stateIds = useMemo(() => results.map(r => r.state_id), [results])

    const { data: rows } = useLiveQuery({
        query: query =>
            stateIds.length === 0
                ? undefined
                : query
                      .from({ s: threadStateCollection.fetchRelations('thread') })
                      .where(({ s }) => inArray(s.id, stateIds))
                      .select(({ s }) => ({
                          ...s,
                          thread_id: s.thread,
                          thread: materialize(
                              query
                                  .from({ t: threadsCollection })
                                  .where(({ t }) => eq(t.id, s.thread))
                                  .findOne()
                          ),
                      })),
    })

    const { data: allAssignments } = useMyLiveQuery((query, { userId }) =>
        query
            .from({ label_assignments: assignmentsCollection })
            .where(({ label_assignments }) =>
                and(
                    eq(label_assignments.collection, 'mail_thread_state'),
                    eq(label_assignments.user, userId)
                )
            )
    )

    const rowById = useMemo(() => {
        const map = new Map<string, ThreadListRow>()
        for (const row of (rows ?? []) as ThreadListRow[]) map.set(row.id, row)
        return map
    }, [rows])

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
        for (const hit of results) {
            const row = rowById.get(hit.state_id)
            if (!row) continue
            const labels = (labelIdsByRecord.get(row.id) ?? [])
                .map(id => labelMap.get(id))
                .filter(isLabel)
            const snippet = stripHtmlTags(hit.snippet_highlight) || undefined
            out.push(toThreadListItem(row, labels, { snippet }))
        }
        return out
    }, [results, rowById, labelIdsByRecord, labelMap])
}
