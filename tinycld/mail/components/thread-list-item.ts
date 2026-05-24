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
    mailboxLabel?: string
}

export function toThreadListItem(
    state: MailThreadState,
    thread: MailThreads | undefined,
    labels: LabelInfo[],
    hasDraft = false,
    hasAttachments = false,
    mailboxLabel?: string
): ThreadListItem {
    const t = thread
    const participants = t?.participants ?? []
    const firstSender = participants[0]
    const senderEmail = firstSender?.email ?? ''

    return {
        stateId: state.id,
        threadId: t?.id ?? state.thread,
        subject: t?.subject ?? '',
        snippet: t?.snippet ?? '',
        latestDate: t?.latest_date ?? state.updated,
        messageCount: t?.message_count ?? 1,
        // Fall back to the email address when the sender has no display name,
        // so the list never shows a blank sender column (and avatar initials
        // derive from something). Detail view keeps name + email separate.
        senderName: firstSender?.name || senderEmail,
        senderEmail,
        participants,
        isRead: state.is_read,
        isStarred: state.is_starred,
        labels,
        folder: state.folder,
        hasDraft,
        hasAttachments,
        mailboxLabel,
    }
}
