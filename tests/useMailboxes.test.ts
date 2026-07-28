import { describe, expect, it } from 'vitest'
import { splitMailboxes } from '~/tinycld/mail/hooks/splitMailboxes'
import { getMailboxLabel } from '~/tinycld/mail/hooks/useMailboxes'
import type { MailMailboxes } from '~/tinycld/mail/types'

function makeMailbox(overrides: Partial<MailMailboxes>): MailMailboxes {
    return {
        id: 'mb1',
        address: 'alice',
        domain: 'd1',
        display_name: 'Alice',
        name: 'Acme',
        type: 'personal',
        created: '2024-01-01T00:00:00Z',
        updated: '2024-01-01T00:00:00Z',
        ...overrides,
    }
}

describe('splitMailboxes', () => {
    it('returns the personal mailbox as `personal`', () => {
        const mb = makeMailbox({ id: 'mb1', type: 'personal' })
        const got = splitMailboxes(['mb1'], [mb])
        expect(got.personal?.id).toBe('mb1')
        expect(got.shared).toEqual([])
    })

    it('separates personal from shared mailboxes', () => {
        const personal = makeMailbox({ id: 'mb1', type: 'personal', address: 'alice' })
        const shared = makeMailbox({ id: 'mb2', type: 'shared', address: 'support' })
        const got = splitMailboxes(['mb1', 'mb2'], [personal, shared])
        expect(got.personal?.id).toBe('mb1')
        expect(got.shared.map(m => m.id)).toEqual(['mb2'])
    })

    it('excludes mailboxes the user is not a member of', () => {
        const personal = makeMailbox({ id: 'mb1', type: 'personal' })
        const shared = makeMailbox({ id: 'mb2', type: 'shared' })
        const got = splitMailboxes(['mb1'], [personal, shared])
        expect(got.shared).toEqual([])
    })

    it('sorts shared mailboxes by created ascending', () => {
        const early = makeMailbox({ id: 'mb_a', type: 'shared', created: '2024-01-01T00:00:00Z' })
        const late = makeMailbox({ id: 'mb_b', type: 'shared', created: '2024-06-01T00:00:00Z' })
        const got = splitMailboxes(['mb_a', 'mb_b'], [late, early])
        expect(got.shared.map(m => m.id)).toEqual(['mb_a', 'mb_b'])
    })

    it('returns null personal when user has no personal mailbox', () => {
        const shared = makeMailbox({ id: 'mb2', type: 'shared' })
        const got = splitMailboxes(['mb2'], [shared])
        expect(got.personal).toBeNull()
        expect(got.shared).toHaveLength(1)
    })
})

describe('getMailboxLabel', () => {
    it('returns "Personal" for the personal mailbox regardless of display_name', () => {
        const mb = makeMailbox({ display_name: 'Alice', address: 'alice@x' })
        expect(getMailboxLabel(mb, true)).toBe('Personal')
    })

    it('returns display_name for shared mailboxes when set', () => {
        const mb = makeMailbox({
            type: 'shared',
            display_name: 'Team Support',
            address: 'support@x',
        })
        expect(getMailboxLabel(mb, false)).toBe('Team Support')
    })

    it('falls back to address when display_name is empty for shared mailboxes', () => {
        const mb = makeMailbox({ type: 'shared', display_name: '', address: 'support@x' })
        expect(getMailboxLabel(mb, false)).toBe('support@x')
    })
})
