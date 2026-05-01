import { and, eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { useEffect, useMemo, useRef } from 'react'
import type { ThreadListItem } from '../components/thread-list-item'
import { toThreadListItem } from '../components/thread-list-item'
import { useThreadListStore } from '../stores/thread-list-store'
import type { MailMessages, MailThreadState } from '../types'
import { mergeSharedFolderStates } from './mergeSharedFolderStates'
import { useLabels } from './useLabels'
import { getMailboxLabel } from './useMailboxes'

export const UNIFIED_INBOX = '__all_inboxes__'

export function useThreadListItems(
    userOrgId: string,
    filter: { folder: string | null; labels: string[]; mailboxId: string }
) {
    const [
        threadStateCollection,
        threadsCollection,
        messagesCollection,
        assignmentsCollection,
        mailboxesCollection,
        domainsCollection,
        membersCollection,
    ] = useStore(
        'mail_thread_state',
        'mail_threads',
        'mail_messages',
        'label_assignments',
        'mail_mailboxes',
        'mail_domains',
        'mail_mailbox_members'
    )

    const { labels, labelMap } = useLabels()

    const { data: threadStates, isLoading: threadStatesLoading } = useOrgLiveQuery(
        (query) =>
            query
                .from({ mail_thread_state: threadStateCollection })
                .where(({ mail_thread_state }) => eq(mail_thread_state.user_org, userOrgId)),
        [userOrgId]
    )

    const { data: threads, isLoading: threadsLoading } = useOrgLiveQuery((query, { orgId }) =>
        query
            .from({ t: threadsCollection })
            .join({ mb: mailboxesCollection }, ({ t, mb }) => eq(t.mailbox, mb.id))
            .join({ d: domainsCollection }, ({ mb, d }) => eq(mb.domain, d.id))
            .where(({ d }) => eq(d.org, orgId))
            .select(({ t }) => t)
    )

    const { data: draftMessages, isLoading: draftMessagesLoading } = useOrgLiveQuery((query, { orgId }) =>
        query
            .from({ msg: messagesCollection })
            .join({ t: threadsCollection }, ({ msg, t }) => eq(msg.thread, t.id))
            .join({ mb: mailboxesCollection }, ({ t, mb }) => eq(t.mailbox, mb.id))
            .join({ d: domainsCollection }, ({ mb, d }) => eq(mb.domain, d.id))
            .where(({ msg, d }) => and(eq(d.org, orgId), eq(msg.delivery_status, 'draft')))
            .select(({ msg }) => msg)
    )

    const { data: attachmentMessages, isLoading: attachmentMessagesLoading } = useOrgLiveQuery((query, { orgId }) =>
        query
            .from({ msg: messagesCollection })
            .join({ t: threadsCollection }, ({ msg, t }) => eq(msg.thread, t.id))
            .join({ mb: mailboxesCollection }, ({ t, mb }) => eq(t.mailbox, mb.id))
            .join({ d: domainsCollection }, ({ mb, d }) => eq(mb.domain, d.id))
            .where(({ msg, d }) => and(eq(d.org, orgId), eq(msg.has_attachments, true)))
            .select(({ msg }) => msg)
    )

    const { data: allAssignments, isLoading: assignmentsLoading } = useOrgLiveQuery((query, { userOrgId }) =>
        query
            .from({ label_assignments: assignmentsCollection })
            .where(({ label_assignments }) =>
                and(eq(label_assignments.collection, 'mail_thread_state'), eq(label_assignments.user_org, userOrgId))
            )
    )

    const { data: targetMailbox } = useOrgLiveQuery(
        (query) =>
            query
                .from({ mail_mailboxes: mailboxesCollection })
                .where(({ mail_mailboxes }) => eq(mail_mailboxes.id, filter.mailboxId)),
        [filter.mailboxId]
    )
    const mailboxType = targetMailbox?.[0]?.type ?? 'personal'

    const { data: coMembers } = useOrgLiveQuery(
        (query) =>
            query
                .from({ mail_mailbox_members: membersCollection })
                .where(({ mail_mailbox_members }) => eq(mail_mailbox_members.mailbox, filter.mailboxId)),
        [filter.mailboxId]
    )

    const needsSharedTeamStates = mailboxType === 'shared' && (filter.folder === 'sent' || filter.folder === 'drafts')

    const { data: sharedFolderStates } = useOrgLiveQuery(
        (query) =>
            query
                .from({ mail_thread_state: threadStateCollection })
                .where(({ mail_thread_state }) => eq(mail_thread_state.folder, filter.folder ?? 'sent')),
        [filter.folder]
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
        const map = new Map<string, NonNullable<typeof threads>[number]>()
        for (const t of threads ?? []) {
            map.set(t.id, t)
        }
        return map
    }, [threads])

    const { data: allMailboxes } = useOrgLiveQuery((query) => query.from({ mail_mailboxes: mailboxesCollection }))

    const { data: userMemberships } = useOrgLiveQuery((query, { userOrgId }) =>
        query
            .from({ mail_mailbox_members: membersCollection })
            .where(({ mail_mailbox_members }) => eq(mail_mailbox_members.user_org, userOrgId))
    )

    const isUnified = filter.mailboxId === UNIFIED_INBOX

    const mailboxLabelMap = useMemo(() => {
        if (!isUnified || !allMailboxes || !userMemberships) return null
        const myMailboxIds = new Set(userMemberships.map((m) => m.mailbox))
        const map = new Map<string, string>()
        for (const mb of allMailboxes) {
            if (!myMailboxIds.has(mb.id)) continue
            map.set(mb.id, getMailboxLabel(mb, mb.type === 'personal'))
        }
        return map
    }, [isUnified, allMailboxes, userMemberships])

    const items: ThreadListItem[] = useMemo(() => {
        if (!threadStates) return []

        const threadIsInMailbox = (threadId: string): boolean => {
            const t = threadMap.get(threadId)
            if (!t) return false
            if (isUnified) return true
            return t.mailbox === filter.mailboxId
        }

        let baseStates: MailThreadState[] = threadStates as MailThreadState[]

        if (needsSharedTeamStates && coMembers && sharedFolderStates) {
            const coMemberIds = coMembers.map((m) => m.user_org)
            baseStates = mergeSharedFolderStates(sharedFolderStates as MailThreadState[], coMemberIds)
        }

        const mapped = baseStates
            .filter((state) => threadIsInMailbox(state.thread))
            .map((state) => {
                const thread = threadMap.get(state.thread)
                const labelIds = assignmentsByRecord.get(state.id) ?? []
                const stateLabels = labelIds
                    .map((id) => labelMap.get(id))
                    .filter((l): l is { id: string; name: string; color: string } => l != null)
                const hasDraft = draftByThread.has(state.thread)
                const hasAttachments = threadsWithAttachments.has(state.thread)
                const mailboxLabel = isUnified && thread ? mailboxLabelMap?.get(thread.mailbox) : undefined
                return toThreadListItem(state, thread, stateLabels, hasDraft, hasAttachments, mailboxLabel)
            })
            .sort((a, b) => new Date(b.latestDate).getTime() - new Date(a.latestDate).getTime())

        const { folder, labels } = filter
        if (labels.length > 0) {
            return mapped.filter((item) => item.labels.some((l) => labels.includes(l.id)))
        }

        const activeFolder = folder ?? 'inbox'
        if (activeFolder === 'starred') {
            return mapped.filter((item) => item.isStarred)
        }
        if (activeFolder === 'all') {
            return mapped
        }
        if (activeFolder === 'inbox' || activeFolder === 'all-inboxes') {
            return mapped.filter((item) => item.folder === 'inbox')
        }

        return mapped.filter((item) => item.folder === activeFolder)
    }, [
        threadStates,
        sharedFolderStates,
        coMembers,
        needsSharedTeamStates,
        threadMap,
        assignmentsByRecord,
        labelMap,
        draftByThread,
        threadsWithAttachments,
        filter,
        isUnified,
        mailboxLabelMap,
    ])

    // First-load gate: any always-needed query still hydrating. Mode-specific
    // queries (sharedFolderStates, mailboxLabelMap inputs) are intentionally
    // excluded — their absence at most omits a label chip on a row, not the
    // row itself.
    const isLoading =
        threadStatesLoading || threadsLoading || draftMessagesLoading || attachmentMessagesLoading || assignmentsLoading

    // Publish the visible thread IDs to the cross-screen store so the
    // conversation detail screen can navigate prev/next within the same scope.
    // Done here (in the data layer) rather than in the screen via
    // useEffect+ref so it stays consistent with the source of truth and
    // doesn't fire from ref-equality false positives.
    const setThreadIds = useThreadListStore((s) => s.setThreadIds)
    const prevIdsKeyRef = useRef('')
    useEffect(() => {
        const ids = items.map((i) => i.threadId)
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
        draftByThread,
        threadMap,
        threadStateCollection,
        isLoading,
    }
}
