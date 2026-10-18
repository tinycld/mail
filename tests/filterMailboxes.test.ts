import { describe, expect, it } from 'vitest'
import { filterAndGroupMailboxes, type MailboxListItem } from '~/tinycld/mail/hooks/filterMailboxes'

function item(overrides: Partial<MailboxListItem> = {}): MailboxListItem {
    return {
        id: 'mb1',
        address: 'support',
        domainName: 'tinycld.org',
        displayName: 'Support Team',
        type: 'shared',
        memberCount: 1,
        otherAddressCount: 0,
        memberNames: ['Nathan Stitt'],
        memberEmails: ['nathan@tinycld.org'],
        otherAddresses: [],
        ...overrides,
    }
}

describe('filterAndGroupMailboxes', () => {
    it('returns only shared inboxes', () => {
        const a = item({ id: 'a', type: 'shared' })
        const b = item({ id: 'b', type: 'personal', address: 'nathan' })
        const got = filterAndGroupMailboxes([a, b], { query: '' })
        expect(got.shared.map(m => m.id)).toEqual(['a'])
        expect(got.totals).toEqual({ shared: 1 })
    })

    it('gives an empty shared list when every inbox is personal', () => {
        const b = item({ id: 'b', type: 'personal' })
        const got = filterAndGroupMailboxes([b], { query: '' })
        expect(got.shared).toEqual([])
        expect(got.totals).toEqual({ shared: 0 })
    })

    it('matches query against primary address', () => {
        const a = item({
            id: 'a',
            address: 'support',
            displayName: 'Help Desk',
            memberNames: [],
            memberEmails: [],
        })
        const b = item({
            id: 'b',
            address: 'billing',
            displayName: 'Help Desk',
            memberNames: [],
            memberEmails: [],
        })
        const got = filterAndGroupMailboxes([a, b], { query: 'sup' })
        expect(got.shared.map(m => m.id)).toEqual(['a'])
    })

    it('matches query against display name (case-insensitive)', () => {
        const a = item({ id: 'a', displayName: 'Support Team' })
        const b = item({ id: 'b', displayName: 'Billing' })
        const got = filterAndGroupMailboxes([a, b], { query: 'TEAM' })
        expect(got.shared.map(m => m.id)).toEqual(['a'])
    })

    it('matches query against member name', () => {
        const a = item({ id: 'a', memberNames: ['Rachel Tan'] })
        const b = item({ id: 'b', memberNames: ['Jamie'] })
        const got = filterAndGroupMailboxes([a, b], { query: 'rachel' })
        expect(got.shared.map(m => m.id)).toEqual(['a'])
    })

    it('matches query against member email', () => {
        const a = item({ id: 'a', memberEmails: ['rachel@tinycld.org'] })
        const b = item({ id: 'b', memberEmails: ['jamie@tinycld.org'] })
        const got = filterAndGroupMailboxes([a, b], { query: 'rachel@' })
        expect(got.shared.map(m => m.id)).toEqual(['a'])
    })

    it('matches query against another address of the inbox', () => {
        const a = item({ id: 'a', otherAddresses: ['help'] })
        const b = item({ id: 'b' })
        const got = filterAndGroupMailboxes([a, b], { query: 'help' })
        expect(got.shared.map(m => m.id)).toEqual(['a'])
    })

    it('sorts alphabetically by address', () => {
        const a = item({ id: 'a', address: 'zeta', type: 'shared' })
        const b = item({ id: 'b', address: 'alpha', type: 'shared' })
        const got = filterAndGroupMailboxes([a, b], { query: '' })
        expect(got.shared.map(m => m.id)).toEqual(['b', 'a'])
    })

    it('totals always reflect the full dataset, not the filtered slice', () => {
        const a = item({ id: 'a', type: 'shared' })
        const b = item({ id: 'b', type: 'shared', address: 'billing', displayName: 'Billing' })
        const c = item({ id: 'c', type: 'personal' })
        const got = filterAndGroupMailboxes([a, b, c], { query: 'billing' })
        expect(got.shared.map(m => m.id)).toEqual(['b'])
        expect(got.totals).toEqual({ shared: 2 })
    })
})
