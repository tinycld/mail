import { describe, expect, it } from 'vitest'
import { moveThreadStateToFolder } from '~/tinycld/mail/lib/thread-folder'

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
