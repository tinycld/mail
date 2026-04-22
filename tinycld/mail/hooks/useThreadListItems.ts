import { and, eq } from '@tanstack/db'
import { useMemo } from 'react'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import type { ThreadListItem } from '../components/thread-list-item'
import { toThreadListItem } from '../components/thread-list-item'
import type { MailMessages, MailThreadState } from '../types'
import { useLabels } from './useLabels'
import { mergeSharedFolderStates } from './mergeSharedFolderStates'

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

    const { data: threadStates } = useOrgLiveQuery(
        (query) =>
            query
                .from({ mail_thread_state: threadStateCollection })
                .where(({ mail_thread_state }) => eq(mail_thread_state.user_org, userOrgId)),
        [userOrgId]
    )

    const { data: threads } = useOrgLiveQuery((query, { orgId }) =>
        query
            .from({ t: threadsCollection })
            .join({ mb: mailboxesCollection }, ({ t, mb }) => eq(t.mailbox, mb.id))
            .join({ d: domainsCollection }, ({ mb, d }) => eq(mb.domain, d.id))
            .where(({ d }) => eq(d.org, orgId))
            .select(({ t }) => t)
    )

    const { data: draftMessages } = useOrgLiveQuery((query, { orgId }) =>
        query
            .from({ msg: messagesCollection })
            .join({ t: threadsCollection }, ({ msg, t }) => eq(msg.thread, t.id))
            .join({ mb: mailboxesCollection }, ({ t, mb }) => eq(t.mailbox, mb.id))
            .join({ d: domainsCollection }, ({ mb, d }) => eq(mb.domain, d.id))
            .where(({ msg, d }) => and(eq(d.org, orgId), eq(msg.delivery_status, 'draft')))
            .select(({ msg }) => msg)
    )

    const { data: attachmentMessages } = useOrgLiveQuery((query, { orgId }) =>
        query
            .from({ msg: messagesCollection })
            .join({ t: threadsCollection }, ({ msg, t }) => eq(msg.thread, t.id))
            .join({ mb: mailboxesCollection }, ({ t, mb }) => eq(t.mailbox, mb.id))
            .join({ d: domainsCollection }, ({ mb, d }) => eq(mb.domain, d.id))
            .where(({ msg, d }) => and(eq(d.org, orgId), eq(msg.has_attachments, true)))
            .select(({ msg }) => msg)
    )

    const { data: allAssignments } = useOrgLiveQuery((query, { userOrgId }) =>
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

    const needsSharedTeamStates =
        mailboxType === 'shared' && (filter.folder === 'sent' || filter.folder === 'drafts')

    const { data: sharedFolderStates } = useOrgLiveQuery(
        (query) =>
            query
                .from({ mail_thread_state: threadStateCollection })
                .where(({ mail_thread_state }) =>
                    eq(mail_thread_state.folder, filter.folder ?? 'sent')
                ),
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

    const items: ThreadListItem[] = useMemo(() => {
        if (!threadStates) return []

        const threadIsInMailbox = (threadId: string): boolean => {
            const t = threadMap.get(threadId)
            return !!t && t.mailbox === filter.mailboxId
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
                return toThreadListItem(state, thread, stateLabels, hasDraft, hasAttachments)
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
        if (activeFolder === 'inbox') {
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
    ])

    return {
        items,
        labels,
        labelMap,
        draftByThread,
        threadStateCollection,
    }
}
