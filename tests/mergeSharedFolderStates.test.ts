import { describe, expect, it } from 'vitest'
import { mergeSharedFolderStates } from '~/tinycld/mail/hooks/mergeSharedFolderStates'
import type { MailThreadState } from '~/tinycld/mail/types'

function st(overrides: Partial<MailThreadState>): MailThreadState {
    return {
        id: 's',
        thread: 't',
        user: 'u',
        folder: 'sent',
        is_read: false,
        is_starred: false,
        created: '',
        updated: '',
        ...overrides,
    }
}

describe('mergeSharedFolderStates', () => {
    it('keeps states whose user is a co-member', () => {
        const states = [
            st({ id: 's1', thread: 't1', user: 'u1' }),
            st({ id: 's2', thread: 't2', user: 'u_other' }),
        ]
        const got = mergeSharedFolderStates(states, ['u1', 'u2'])
        expect(got.map(s => s.id)).toEqual(['s1'])
    })

    it('dedupes by thread id — first wins', () => {
        const states = [
            st({ id: 's1', thread: 't1', user: 'u1' }),
            st({ id: 's2', thread: 't1', user: 'u2' }),
        ]
        const got = mergeSharedFolderStates(states, ['u1', 'u2'])
        expect(got.map(s => s.id)).toEqual(['s1'])
    })

    it('returns empty for empty co-member set', () => {
        const states = [st({ id: 's1', thread: 't1', user: 'u1' })]
        const got = mergeSharedFolderStates(states, [])
        expect(got).toEqual([])
    })
})
