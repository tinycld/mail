import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useMemo } from 'react'
import { useStore } from '~/lib/pocketbase'
import type { ThreadListItem } from '../components/thread-list-item'
import { toThreadListItem } from '../components/thread-list-item'
import type { MailMessages } from '../types'
import { useLabels } from './useLabels'

export function useThreadListItems(
    userOrgId: string,
    filter: { folder: string | null; label: string | null }
) {
    const [threadStateCollection, threadsCollection, messagesCollection, assignmentsCollection] =
        useStore('mail_thread_state', 'mail_threads', 'mail_messages', 'label_assignments')

    const { labels, labelMap } = useLabels()

    const { data: threadStates } = useLiveQuery(
        query =>
            query
                .from({ mail_thread_state: threadStateCollection })
                .where(({ mail_thread_state }) => eq(mail_thread_state.user_org, userOrgId))
                .orderBy(({ mail_thread_state }) => mail_thread_state.updated, 'desc'),
        [userOrgId]
    )

    const { data: threads } = useLiveQuery(
        query => query.from({ mail_threads: threadsCollection }),
        []
    )

    const { data: draftMessages } = useLiveQuery(
        query =>
            query
                .from({ mail_messages: messagesCollection })
                .where(({ mail_messages }) => eq(mail_messages.delivery_status, 'draft')),
        []
    )

    const { data: attachmentMessages } = useLiveQuery(
        query =>
            query
                .from({ mail_messages: messagesCollection })
                .where(({ mail_messages }) => eq(mail_messages.has_attachments, true)),
        []
    )

    const { data: allAssignments } = useLiveQuery(
        query =>
            query
                .from({ label_assignments: assignmentsCollection })
                .where(({ label_assignments }) =>
                    eq(label_assignments.collection, 'mail_thread_state')
                ),
        []
    )

    const assignmentsByRecord = useMemo(() => {
        const map = new Map<string, string[]>()
        for (const a of allAssignments ?? []) {
            const existing = map.get(a.record_id)
            if (existing) {
                existing.push(a.label)
            } else {
                map.set(a.record_id, [a.label])
            }
        }
        return map
    }, [allAssignments])

    const draftByThread = useMemo(() => {
        const map = new Map<string, MailMessages>()
        for (const msg of (draftMessages ?? []) as MailMessages[]) {
            map.set(msg.thread, msg)
        }
        return map
    }, [draftMessages])

    const threadsWithAttachments = useMemo(() => {
        const set = new Set<string>()
        for (const msg of (attachmentMessages ?? []) as MailMessages[]) {
            set.add(msg.thread)
        }
        return set
    }, [attachmentMessages])

    const threadMap = useMemo(() => {
        const map = new Map<string, (typeof threads)[number]>()
        for (const t of threads ?? []) {
            map.set(t.id, t)
        }
        return map
    }, [threads])

    const items: ThreadListItem[] = useMemo(() => {
        if (!threadStates) return []

        const mapped = threadStates.map(state => {
            const thread = threadMap.get(state.thread)
            const labelIds = assignmentsByRecord.get(state.id) ?? []
            const stateLabels = labelIds
                .map(id => labelMap.get(id))
                .filter((l): l is { id: string; name: string; color: string } => l != null)
            const hasDraft = draftByThread.has(state.thread)
            const hasAttachments = threadsWithAttachments.has(state.thread)
            return toThreadListItem(state, thread, stateLabels, hasDraft, hasAttachments)
        })

        const { folder, label } = filter
        if (label) {
            return mapped.filter(item => item.labels.some(l => l.id === label))
        }

        const activeFolder = folder ?? 'inbox'
        if (activeFolder === 'starred') {
            return mapped.filter(item => item.isStarred)
        }
        if (activeFolder === 'all') {
            return mapped
        }
        if (activeFolder === 'inbox') {
            return mapped.filter(item => item.folder === 'inbox')
        }

        return mapped.filter(item => item.folder === activeFolder)
    }, [
        threadStates,
        threadMap,
        assignmentsByRecord,
        labelMap,
        draftByThread,
        threadsWithAttachments,
        filter,
    ])

    return {
        items,
        labels,
        labelMap,
        draftByThread,
        threadStateCollection,
    }
}
