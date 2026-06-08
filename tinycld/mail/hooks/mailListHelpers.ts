import { captureException } from '@tinycld/core/lib/errors'
import type { ThreadListItem } from '../components/thread-list-item'
import type { MailThreadState } from '../types'
import type { MailSearchResult } from './useMailSearch'

type LabelInfo = { id: string; name: string; color: string }

const FOLDER_TITLES: Record<string, string> = {
    'all-inboxes': 'All Inboxes',
}

export function stripHtmlTags(html: string): string {
    return html.replace(/<[^>]*>/g, '')
}

export function prettifyFolderKey(key: string): string {
    if (FOLDER_TITLES[key]) return FOLDER_TITLES[key]
    return key
        .split('-')
        .map(part => (part ? part.charAt(0).toUpperCase() + part.slice(1) : part))
        .join(' ')
}

export function parseSearchParticipants(raw: string): { name: string; email: string }[] {
    if (!raw) return []
    try {
        const parsed = JSON.parse(raw)
        return Array.isArray(parsed) ? parsed : []
    } catch (err) {
        captureException('mail.searchResultParticipants', err, { raw })
        return []
    }
}

/**
 * Build a list row from an FTS search hit. The hit carries display data
 * (subject, snippet, participants) but no mail_thread_state id — that lives in
 * the per-user (eager, bounded) thread_state collection. The caller resolves it
 * by thread id and passes it here so the row carries a REAL stateId and the
 * actual read/starred/folder flags. Without a resolved state the row's swipe
 * actions would target `thread_id` as if it were a state id and silently no-op,
 * so callers should drop hits with no state (see useSearchThreadItems).
 */
export function searchResultToThreadListItem(
    result: MailSearchResult,
    state?: MailThreadState,
    labels: LabelInfo[] = []
): ThreadListItem {
    const participants = parseSearchParticipants(result.participants)
    const firstSender = participants[0]
    const senderEmail = firstSender?.email ?? ''
    return {
        // Fall back to thread_id only when no state resolved — keeps the row
        // renderable, but such a row's actions can't work (no real state row).
        stateId: state?.id ?? result.thread_id,
        threadId: result.thread_id,
        subject: result.subject,
        snippet: stripHtmlTags(result.snippet_highlight) || '',
        latestDate: result.latest_date,
        messageCount: result.message_count,
        // Fall back to the email when there's no display name (matches
        // toThreadListItem) so the list sender column is never blank.
        senderName: firstSender?.name || senderEmail,
        senderEmail,
        participants,
        isRead: state?.is_read ?? true,
        isStarred: state?.is_starred ?? false,
        labels,
        folder: state?.folder ?? 'search',
        hasDraft: false,
        hasAttachments: result.has_attachments ?? false,
    }
}
