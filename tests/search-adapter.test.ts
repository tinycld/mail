import { toRow } from '@tinycld/mail/search-adapter'
import { describe, expect, it } from 'vitest'

describe('mail toRow', () => {
    it('maps a hit to a row, using thread_id as the row id', () => {
        const row = toRow({
            thread_id: 't1',
            subject: 'Budget approval',
            subject_highlight: '',
            snippet_highlight: '',
            latest_date: '2026-08-01',
            participants: 'Grace Hopper, Ada Lovelace',
            message_count: 3,
            mailbox_id: 'mb1',
            has_attachments: false,
        })
        expect(row).toEqual({
            id: 't1',
            title: 'Budget approval',
            subtitle: 'Grace Hopper, Ada Lovelace',
            meta: undefined,
        })
    })

    it('falls back to "(no subject)" when subject is empty', () => {
        const row = toRow({
            thread_id: 't2',
            subject: '',
            participants: '',
            latest_date: '',
            mailbox_id: 'mb1',
        })
        expect(row?.title).toBe('(no subject)')
    })
})
