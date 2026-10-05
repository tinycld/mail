import type { MailThreadState, MailThreads } from '../types'

type LabelInfo = { id: string; name: string; color: string }

/**
 * One row of the thread list query: the state row with its thread
 * materialized. The state's relation field is also named `thread`, so the
 * select copies the id into `thread_id` and puts the materialized row under
 * `thread`.
 */
export interface ThreadListRow extends Omit<MailThreadState, 'thread'> {
    thread_id: string
    thread: MailThreads | undefined
}

export interface ThreadListItem {
    stateId: string
    threadId: string
    mailboxId: string
    subject: string
    snippet: string
    latestDate: string
    messageCount: number
    senderName: string
    senderEmail: string
    participants: { name: string; email: string }[]
    isRead: boolean
    isStarred: boolean
    labels: LabelInfo[]
    folder: string
    hasDraft: boolean
    hasAttachments: boolean
    mailboxLabel?: string
}

export function toThreadListItem(
    row: ThreadListRow,
    labels: LabelInfo[],
    options: { mailboxLabel?: string; snippet?: string } = {}
): ThreadListItem {
    const t = row.thread
    const participants = t?.participants ?? []
    const firstSender = participants[0]
    const senderEmail = firstSender?.email ?? ''

    return {
        stateId: row.id,
        threadId: row.thread_id,
        mailboxId: row.mailbox,
        subject: t?.subject ?? '',
        snippet: options.snippet ?? t?.snippet ?? '',
        latestDate: row.latest_date,
        messageCount: t?.message_count ?? 1,
        // Fall back to the address when the sender has no display name, so the
        // list never shows a blank sender column.
        senderName: firstSender?.name || senderEmail,
        senderEmail,
        participants,
        isRead: row.is_read,
        isStarred: row.is_starred,
        labels,
        folder: row.folder,
        hasDraft: t?.has_draft ?? false,
        hasAttachments: t?.has_attachments ?? false,
        mailboxLabel: options.mailboxLabel,
    }
}
