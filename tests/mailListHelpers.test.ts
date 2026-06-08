import { describe, expect, it, vi } from 'vitest'
import {
    parseSearchParticipants,
    prettifyFolderKey,
    searchResultToThreadListItem,
    stripHtmlTags,
} from '~/tinycld/mail/hooks/mailListHelpers'

vi.mock('@tinycld/core/lib/errors', () => ({
    captureException: vi.fn(),
}))

describe('prettifyFolderKey', () => {
    it('returns the canonical title when one is registered', () => {
        expect(prettifyFolderKey('all-inboxes')).toBe('All Inboxes')
    })

    it('title-cases unknown single-word keys', () => {
        expect(prettifyFolderKey('inbox')).toBe('Inbox')
        expect(prettifyFolderKey('sent')).toBe('Sent')
    })

    it('title-cases each segment of an unregistered hyphenated key', () => {
        expect(prettifyFolderKey('shared-with-me')).toBe('Shared With Me')
    })

    it('handles an empty key without throwing', () => {
        expect(prettifyFolderKey('')).toBe('')
    })
})

describe('parseSearchParticipants', () => {
    it('parses a JSON-encoded participants array', () => {
        const raw = JSON.stringify([{ name: 'Ada', email: 'ada@example.com' }])
        expect(parseSearchParticipants(raw)).toEqual([{ name: 'Ada', email: 'ada@example.com' }])
    })

    it('returns [] for empty input', () => {
        expect(parseSearchParticipants('')).toEqual([])
    })

    it('returns [] when the JSON parses to a non-array', () => {
        expect(parseSearchParticipants('{"name":"Ada"}')).toEqual([])
        expect(parseSearchParticipants('null')).toEqual([])
    })

    it('returns [] on malformed JSON without throwing', () => {
        expect(parseSearchParticipants('not json')).toEqual([])
    })
})

describe('stripHtmlTags', () => {
    it('removes simple tags', () => {
        expect(stripHtmlTags('hello <b>world</b>')).toBe('hello world')
    })

    it('removes attributed tags', () => {
        expect(stripHtmlTags('<a href="x">link</a>')).toBe('link')
    })
})

describe('searchResultToThreadListItem', () => {
    function buildResult(
        overrides: Partial<Parameters<typeof searchResultToThreadListItem>[0]> = {}
    ) {
        return {
            thread_id: 't1',
            subject: 'Re: budget',
            subject_highlight: 'Re: budget',
            snippet_highlight: 'See the <em>numbers</em> attached.',
            latest_date: '2026-04-30T12:00:00Z',
            message_count: 3,
            mailbox_id: 'mb1',
            participants: JSON.stringify([
                { name: 'Ada', email: 'ada@example.com' },
                { name: 'Grace', email: 'grace@example.com' },
            ]),
            has_attachments: false,
            ...overrides,
        }
    }

    it('maps a well-formed search result onto a ThreadListItem', () => {
        const item = searchResultToThreadListItem(buildResult())
        expect(item.threadId).toBe('t1')
        expect(item.stateId).toBe('t1')
        expect(item.subject).toBe('Re: budget')
        expect(item.snippet).toBe('See the numbers attached.')
        expect(item.messageCount).toBe(3)
        expect(item.senderName).toBe('Ada')
        expect(item.senderEmail).toBe('ada@example.com')
        expect(item.participants).toHaveLength(2)
        expect(item.folder).toBe('search')
        expect(item.isRead).toBe(true)
    })

    it('survives malformed participants JSON with empty sender fields', () => {
        const item = searchResultToThreadListItem(buildResult({ participants: 'not json' }))
        expect(item.senderName).toBe('')
        expect(item.senderEmail).toBe('')
        expect(item.participants).toEqual([])
    })

    it('falls back to the email when the sender has no display name', () => {
        const item = searchResultToThreadListItem(
            buildResult({
                participants: JSON.stringify([{ name: '', email: 'noreply@tinycld.org' }]),
            })
        )
        expect(item.senderName).toBe('noreply@tinycld.org')
        expect(item.senderEmail).toBe('noreply@tinycld.org')
    })

    it('falls back to empty snippet when stripping leaves nothing', () => {
        const item = searchResultToThreadListItem(buildResult({ snippet_highlight: '<br/>' }))
        expect(item.snippet).toBe('')
    })

    it('forwards has_attachments so the paperclip indicator renders for matched threads with attachments', () => {
        expect(
            searchResultToThreadListItem(buildResult({ has_attachments: true })).hasAttachments
        ).toBe(true)
        expect(
            searchResultToThreadListItem(buildResult({ has_attachments: false })).hasAttachments
        ).toBe(false)
    })

    it('uses the resolved thread_state id (not thread_id) so row actions hit the real record', () => {
        // The whole point of the search-archive fix: when a state row is
        // resolved, stateId must be the STATE id, not the thread id — otherwise
        // archive/star/trash target a non-existent record and silently no-op.
        const state = {
            id: 'state_abc',
            thread: 't1',
            is_read: false,
            is_starred: true,
            folder: 'archive',
            // biome-ignore lint/suspicious/noExplicitAny: test stub of a MailThreadState; only the read fields matter
        } as any
        const item = searchResultToThreadListItem(buildResult(), state)
        expect(item.stateId).toBe('state_abc')
        expect(item.threadId).toBe('t1')
    })

    it('reflects the real read / starred / folder flags from the resolved state', () => {
        const state = {
            id: 'state_abc',
            thread: 't1',
            is_read: false,
            is_starred: true,
            folder: 'archive',
            // biome-ignore lint/suspicious/noExplicitAny: test stub of a MailThreadState
        } as any
        const item = searchResultToThreadListItem(buildResult(), state)
        expect(item.isRead).toBe(false)
        expect(item.isStarred).toBe(true)
        expect(item.folder).toBe('archive')
    })

    it('passes resolved labels through to the row', () => {
        const state = {
            id: 'state_abc',
            thread: 't1',
            is_read: true,
            is_starred: false,
            folder: 'inbox',
            // biome-ignore lint/suspicious/noExplicitAny: test stub of a MailThreadState
        } as any
        const labels = [{ id: 'l1', name: 'Work', color: '#3b82f6' }]
        const item = searchResultToThreadListItem(buildResult(), state, labels)
        expect(item.labels).toEqual(labels)
    })
})
