import { describe, expect, it } from 'vitest'
import { moveThreadStateToFolder } from '~/tinycld/mail/lib/thread-folder'
import { buildThreadsFilter } from '~/tinycld/mail/lib/threads-filter'

const base = { mailboxIds: ['mb1'], userIds: ['u1'] }

describe('buildThreadsFilter', () => {
    it('filters the inbox by folder', () => {
        const filter = buildThreadsFilter({ ...base, folder: 'inbox' })
        expect(filter).toContain('mail_thread_state_via_thread.folder ?= "inbox"')
    })

    // A reply leaves its thread in the Inbox and flags it sent, so Sent must
    // read the flag; filtering on folder='sent' hid every replied-to thread.
    it('filters sent by the is_sent flag, not the folder', () => {
        const filter = buildThreadsFilter({ ...base, folder: 'sent' })
        expect(filter).toContain('mail_thread_state_via_thread.is_sent ?= true')
        expect(filter).not.toContain('folder ?= "sent"')
    })

    it('keeps trashed and spam threads out of sent', () => {
        const filter = buildThreadsFilter({ ...base, folder: 'sent' })
        expect(filter).toContain('mail_thread_state_via_thread.folder ?!= "trash"')
        expect(filter).toContain('mail_thread_state_via_thread.folder ?!= "spam"')
    })
})

describe('moveThreadStateToFolder', () => {
    it('flags a thread moved to sent so the Sent view lists it', () => {
        const draft = { folder: 'inbox' as const, is_sent: false }
        moveThreadStateToFolder(draft, 'sent')
        expect(draft).toEqual({ folder: 'sent', is_sent: true })
    })

    it('keeps the sent flag when a thread moves elsewhere', () => {
        const draft = { folder: 'inbox' as const, is_sent: true }
        moveThreadStateToFolder(draft, 'archive')
        expect(draft).toEqual({ folder: 'archive', is_sent: true })
    })
})
