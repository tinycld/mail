import { and, eq, inArray } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { materialize } from 'pbtsdb'
import { useEffect, useMemo, useRef } from 'react'
import {
    type ThreadListItem,
    type ThreadListRow,
    toThreadListItem,
} from '../components/thread-list-item'
import { cursorTerms, folderTerms, type ThreadCursor } from '../lib/thread-list-query'
import { useThreadListStore } from '../stores/thread-list-store'
import { useLabels } from './useLabels'
import { getMailboxLabel, useMailboxes } from './useMailboxes'

export const UNIFIED_INBOX = '__all_inboxes__'

export const PAGE_SIZE = 100

interface UseThreadListItemsFilter {
    folder: string | null
    labels: string[]
    mailboxId: string
    /** The last row of the previous page, or null on page one. */
    cursor: ThreadCursor | null
}

type LabelInfo = { id: string; name: string; color: string }

// Stable while the query is disabled, so the items memo does not rerun.
const EMPTY_ROWS: ThreadListRow[] = []
const isLabel = (l: LabelInfo | undefined): l is LabelInfo => l != null

/**
 * The thread list is one live query on mail_thread_state with the thread
 * expanded: one request per page (`filter`, `sort=-latest_date,-id`,
 * `perPage=100`, `expand=thread`). mailbox and latest_date are denormalized
 * onto the state row by the server (thread_state_sync.go), because a live
 * query cannot sort or filter through a relation.
 *
 * Realtime subscribes to the where. Page one has no cursor and receives every
 * new row; a deeper page subscribes to its slice. Filed threads are held live,
 * so a new message in a listed thread updates its snippet in place.
 *
 * Label assignments stay one session-scoped query indexed by record id: a
 * materialize per row would cost one request per row, and a join yields one
 * row per assignment.
 */
export function useThreadListItems(filter: UseThreadListItemsFilter) {
    const [threadStateCollection, threadsCollection, assignmentsCollection] = useStore(
        'mail_thread_state',
        'mail_threads',
        'label_assignments'
    )

    const { labels, labelMap } = useLabels()
    const { personal, shared, isLoading: mailboxesLoading } = useMailboxes()
    const isUnified = filter.mailboxId === UNIFIED_INBOX

    const visibleMailboxIds = useMemo(() => {
        if (!isUnified) return filter.mailboxId ? [filter.mailboxId] : []
        const ids: string[] = []
        if (personal) ids.push(personal.id)
        for (const mb of shared) ids.push(mb.id)
        return ids
    }, [isUnified, filter.mailboxId, personal, shared])

    const { folder, cursor } = filter
    const { data: rows, isLoading: rowsLoading } = useMyLiveQuery((query, { userId }) =>
        visibleMailboxIds.length === 0
            ? null
            : query
                  .from({ s: threadStateCollection.fetchRelations('thread') })
                  .where(({ s }) =>
                      and(
                          eq(s.user, userId),
                          inArray(s.mailbox, visibleMailboxIds),
                          ...folderTerms(s, folder),
                          ...cursorTerms(s, cursor)
                      )
                  )
                  .orderBy(({ s }) => s.latest_date, 'desc')
                  .orderBy(({ s }) => s.id, 'desc')
                  .limit(PAGE_SIZE)
                  .select(({ s }) => ({
                      ...s,
                      thread_id: s.thread,
                      thread: materialize(
                          query
                              .from({ t: threadsCollection })
                              .where(({ t }) => eq(t.id, s.thread))
                              .findOne()
                      ),
                  }))
    )

    const { data: allAssignments, isLoading: assignmentsLoading } = useMyLiveQuery(
        (query, { userId }) =>
            query
                .from({ label_assignments: assignmentsCollection })
                .where(({ label_assignments }) =>
                    and(
                        eq(label_assignments.collection, 'mail_thread_state'),
                        eq(label_assignments.user, userId)
                    )
                )
    )

    const assignmentsByRecord = useMemo(() => {
        const map = new Map<string, string[]>()
        for (const a of allAssignments ?? []) {
            const list = map.get(a.record_id) ?? []
            list.push(a.label)
            map.set(a.record_id, list)
        }
        return map
    }, [allAssignments])

    const mailboxLabelMap = useMemo(() => {
        if (!isUnified) return null
        const map = new Map<string, string>()
        if (personal) map.set(personal.id, getMailboxLabel(personal, true))
        for (const mb of shared) map.set(mb.id, getMailboxLabel(mb, false))
        return map
    }, [isUnified, personal, shared])

    const pageRows: ThreadListRow[] = rows ?? EMPTY_ROWS

    const items: ThreadListItem[] = useMemo(() => {
        const out = pageRows.map(row => {
            const rowLabels = (assignmentsByRecord.get(row.id) ?? [])
                .map(id => labelMap.get(id))
                .filter(isLabel)
            return toThreadListItem(row, rowLabels, {
                mailboxLabel: mailboxLabelMap?.get(row.mailbox),
            })
        })
        if (filter.labels.length === 0) return out
        return out.filter(item => filter.labels.every(id => item.labels.some(l => l.id === id)))
    }, [pageRows, assignmentsByRecord, labelMap, mailboxLabelMap, filter.labels])

    // The boundary for the next page is the last row of this one, before the
    // client-side label filter, so paging walks the server order.
    const last = pageRows[pageRows.length - 1]
    const nextCursor: ThreadCursor | null = last ? { date: last.latest_date, id: last.id } : null

    // The query is disabled until a mailbox is known, and a disabled query
    // reports not-loading; without this the empty state flashes on cold load.
    const itemsLoading = rowsLoading || (visibleMailboxIds.length === 0 && mailboxesLoading)

    const setThreadIds = useThreadListStore(s => s.setThreadIds)
    const prevIdsKeyRef = useRef('')
    useEffect(() => {
        const ids = items.map(i => i.threadId)
        const key = ids.join(',')
        if (key !== prevIdsKeyRef.current) {
            prevIdsKeyRef.current = key
            setThreadIds(ids)
        }
    }, [items, setThreadIds])

    return {
        items,
        labels,
        labelMap,
        threadStateCollection,
        visibleMailboxIds,
        isLoading: itemsLoading || assignmentsLoading,
        itemsLoading,
        nextCursor,
        hasFullPage: pageRows.length === PAGE_SIZE,
    }
}
