import type { MailThreadState, MailThreads } from '../types'

type LabelInfo = { id: string; name: string; color: string }

export interface ThreadListItem {
    stateId: string
    threadId: string
    subject: string
    snippet: string
    latestDate: string
    messageCount: number
    senderName: string
    senderEmail: string
    participants: { name: string; email: string }[]
    isRead: boolean
    isStarred: boolean
    isImportant: boolean
    labels: { id: string; name: string; color: string }[]
    folder: string
    hasDraft: boolean
    hasAttachments: boolean
}

export function toThreadListItem(
    state: MailThreadState,
    thread: MailThreads | undefined,
    labels: LabelInfo[],
    hasDraft = false,
    hasAttachments = false
): ThreadListItem {
    const t = thread
    const participants = t?.participants ?? []

    return {
        stateId: state.id,
        threadId: t?.id ?? state.thread,
        subject: t?.subject ?? '',
        snippet: t?.snippet ?? '',
        latestDate: t?.latest_date ?? state.updated,
        messageCount: t?.message_count ?? 1,
        senderName: participants[0]?.name ?? '',
        senderEmail: participants[0]?.email ?? '',
        participants,
        isRead: state.is_read,
        isStarred: state.is_starred,
        isImportant: state.is_important,
        labels,
        folder: state.folder,
        hasDraft,
        hasAttachments,
    }
}
