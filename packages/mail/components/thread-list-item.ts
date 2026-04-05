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
        labels,
        folder: state.folder,
        hasDraft,
        hasAttachments,
    }
}

export function formatMailDate(isoDate: string): string {
    if (!isoDate) return ''

    const date = new Date(isoDate)
    const now = new Date()

    if (Number.isNaN(date.getTime())) return isoDate

    const isToday =
        date.getFullYear() === now.getFullYear() &&
        date.getMonth() === now.getMonth() &&
        date.getDate() === now.getDate()

    if (isToday) {
        return date.toLocaleTimeString(undefined, {
            hour: 'numeric',
            minute: '2-digit',
        })
    }

    const yesterday = new Date(now)
    yesterday.setDate(yesterday.getDate() - 1)
    const isYesterday =
        date.getFullYear() === yesterday.getFullYear() &&
        date.getMonth() === yesterday.getMonth() &&
        date.getDate() === yesterday.getDate()

    if (isYesterday) return 'Yesterday'

    if (date.getFullYear() === now.getFullYear()) {
        return date.toLocaleDateString(undefined, {
            month: 'short',
            day: 'numeric',
        })
    }

    return date.toLocaleDateString(undefined, {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
    })
}
