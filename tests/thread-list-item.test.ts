import { describe, expect, it } from 'vitest'
import { type ThreadListRow, toThreadListItem } from '~/tinycld/mail/components/thread-list-item'

const row: ThreadListRow = {
    id: 'state1',
    thread_id: 'thread1',
    user: 'u1',
    mailbox: 'mb1',
    folder: 'inbox',
    is_read: false,
    is_starred: true,
    is_sent: false,
    latest_date: '2026-10-05 10:00:00.000Z',
    created: '',
    updated: '',
    thread: {
        id: 'thread1',
        mailbox: 'mb1',
        subject: 'Hello',
        snippet: 'stored snippet',
        message_count: 3,
        latest_date: '2026-10-05 10:00:00.000Z',
        participants: [{ name: '', email: 'ann@example.com' }],
        has_draft: true,
        has_attachments: false,
        created: '',
        updated: '',
    },
}

describe('toThreadListItem', () => {
    it('maps the selected row, falling back to the address for a nameless sender', () => {
        const item = toThreadListItem(row, [], { mailboxLabel: 'Support' })
        expect(item).toMatchObject({
            stateId: 'state1',
            threadId: 'thread1',
            mailboxId: 'mb1',
            subject: 'Hello',
            snippet: 'stored snippet',
            senderName: 'ann@example.com',
            senderEmail: 'ann@example.com',
            messageCount: 3,
            isRead: false,
            isStarred: true,
            folder: 'inbox',
            hasDraft: true,
            hasAttachments: false,
            mailboxLabel: 'Support',
        })
    })

    it('tolerates a thread that is not in the store yet', () => {
        const item = toThreadListItem({ ...row, thread: undefined }, [])
        expect(item.threadId).toBe('thread1')
        expect(item.subject).toBe('')
        expect(item.messageCount).toBe(1)
        expect(item.hasDraft).toBe(false)
    })

    it('a snippet override replaces the stored snippet', () => {
        expect(toThreadListItem(row, [], { snippet: 'hit text' }).snippet).toBe('hit text')
    })
})
