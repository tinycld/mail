import { describe, expect, it } from 'vitest'
import {
    addressesLost,
    inboxesLost,
    removalNoticeText,
} from '~/tinycld/mail/settings/group-removal-notice'

describe('removalNoticeText', () => {
    it('names the one address a person loses', () => {
        expect(removalNoticeText({ personName: 'Ann', addresses: ['ann@q.example'] })).toBe(
            'Ann loses the address ann@q.example.'
        )
    })

    it('lists the addresses a person loses', () => {
        expect(removalNoticeText({ personName: 'Ann', addresses: ['a', 'b'] })).toBe(
            'Ann loses these addresses: a, b.'
        )
    })

    it('counts the addresses members lose when the group is deleted', () => {
        expect(removalNoticeText({ personName: null, addresses: ['a', 'b', 'c'] })).toBe(
            'Members lose 3 addresses: a, b, c.'
        )
    })

    it('names the one address members lose when the group is deleted', () => {
        expect(removalNoticeText({ personName: null, addresses: ['a'] })).toBe(
            'Members lose the address a.'
        )
    })

    it('names only the first five addresses', () => {
        const addresses = ['a', 'b', 'c', 'd', 'e', 'f', 'g']
        expect(removalNoticeText({ personName: null, addresses })).toBe(
            'Members lose 7 addresses: a, b, c, d, e and 2 more.'
        )
    })

    it('says nothing when nothing is lost', () => {
        expect(removalNoticeText({ personName: 'Ann', addresses: [] })).toBeNull()
        expect(removalNoticeText({ personName: null, addresses: [] })).toBeNull()
    })
})

describe('removalNoticeText with inboxes', () => {
    it('names an address and an inbox a person loses', () => {
        expect(
            removalNoticeText({
                personName: 'Holly',
                addresses: ['holly@q.example'],
                inboxes: ['support@q.example'],
            })
        ).toBe(
            'Holly loses the address holly@q.example. Holly also loses access to support@q.example.'
        )
    })

    it('names only inboxes', () => {
        expect(
            removalNoticeText({
                personName: 'Holly',
                addresses: [],
                inboxes: ['support@q.example'],
            })
        ).toBe('Holly loses access to support@q.example.')
    })

    it('counts the inboxes members lose when the group is deleted', () => {
        expect(
            removalNoticeText({
                personName: null,
                addresses: [],
                inboxes: ['support@q.example', 'sales@q.example'],
            })
        ).toBe('Members lose access to 2 shared inboxes: support@q.example, sales@q.example.')
    })

    // One list per sentence: a long address list ends in "and 2 more", so a
    // second "and" in the same sentence would be unclear.
    it('names several addresses and an inbox of a person in two sentences', () => {
        expect(
            removalNoticeText({
                personName: 'Ann',
                addresses: ['a', 'b', 'c', 'd', 'e', 'f', 'g'],
                inboxes: ['support@q.example'],
            })
        ).toBe(
            'Ann loses these addresses: a, b, c, d, e and 2 more. Ann also loses access to support@q.example.'
        )
    })

    it('names several inboxes of a person', () => {
        expect(
            removalNoticeText({
                personName: 'Ann',
                addresses: ['a'],
                inboxes: ['sales@q.example', 'support@q.example'],
            })
        ).toBe(
            'Ann loses the address a. Ann also loses access to these shared inboxes: sales@q.example, support@q.example.'
        )
    })

    it('counts the addresses and inboxes members lose when the group is deleted', () => {
        expect(
            removalNoticeText({
                personName: null,
                addresses: ['a', 'b'],
                inboxes: ['sales@q.example', 'support@q.example'],
            })
        ).toBe(
            'Members lose 2 addresses: a, b. Members also lose access to 2 shared inboxes: sales@q.example, support@q.example.'
        )
    })

    it('says nothing without addresses and inboxes', () => {
        expect(removalNoticeText({ personName: 'Ann', addresses: [], inboxes: [] })).toBeNull()
    })
})

describe('inboxesLost', () => {
    const row = (mailboxId: string, userId: string, viaGroup: string) => ({
        mailboxId,
        email: `${mailboxId}@q.example`,
        userId,
        viaGroup,
    })

    it('loses an inbox when only the derived row of the group is left', () => {
        expect(inboxesLost([row('support', 'ann', 'g_sales')], 'g_sales')).toEqual([
            'support@q.example',
        ])
    })

    it('keeps an inbox the person also has by a direct row', () => {
        const rows = [row('support', 'ann', 'g_sales'), row('support', 'ann', '')]
        expect(inboxesLost(rows, 'g_sales')).toEqual([])
    })

    it('keeps an inbox the person has through another group', () => {
        const rows = [row('support', 'ann', 'g_sales'), row('support', 'ann', 'g_ops')]
        expect(inboxesLost(rows, 'g_sales')).toEqual([])
    })

    it('ignores a person without a derived row of the group', () => {
        expect(inboxesLost([row('support', 'bob', '')], 'g_sales')).toEqual([])
    })

    it('lists each lost inbox once, sorted, when members of a deleted group lose it', () => {
        const rows = [
            row('support', 'ann', 'g_sales'),
            row('support', 'bob', 'g_sales'),
            row('sales', 'bob', 'g_sales'),
            row('sales', 'bob', ''),
            row('sales', 'ann', 'g_sales'),
        ]
        expect(inboxesLost(rows, 'g_sales')).toEqual(['sales@q.example', 'support@q.example'])
    })
})

describe('addressesLost', () => {
    const row = (email: string, userId: string, coverageGroups: string[]) => ({
        email,
        userId,
        coverageGroups,
    })

    it('loses an address the removed group gave', () => {
        const rows = [row('ann@q.example', 'ann', ['g_sales'])]
        const memberships = new Map([['ann', ['g_sales']]])
        expect(addressesLost(rows, memberships, 'g_sales')).toEqual(['ann@q.example'])
    })

    // The server moves via_group to another of the domain's groups the person
    // is still in, and keeps the address.
    it('keeps an address while the person is in another of the domain groups', () => {
        const rows = [row('ann@q.example', 'ann', ['g_sales', 'g_ops'])]
        const memberships = new Map([['ann', ['g_sales', 'g_ops']]])
        expect(addressesLost(rows, memberships, 'g_sales')).toEqual([])
    })

    it('loses the address when the other group is not one of the domain groups', () => {
        const rows = [row('ann@q.example', 'ann', ['g_sales'])]
        const memberships = new Map([['ann', ['g_sales', 'g_ops']]])
        expect(addressesLost(rows, memberships, 'g_sales')).toEqual(['ann@q.example'])
    })

    it('applies the same per member when the group is deleted', () => {
        const rows = [
            row('bob@q.example', 'bob', ['g_sales', 'g_ops']),
            row('ann@q.example', 'ann', ['g_sales', 'g_ops']),
            row('ann@r.example', 'ann', ['g_sales']),
            row('cat@q.example', 'cat', ['g_sales', 'g_ops']),
        ]
        const memberships = new Map([
            ['ann', ['g_sales', 'g_ops']],
            ['bob', ['g_sales']],
        ])
        expect(addressesLost(rows, memberships, 'g_sales')).toEqual([
            'ann@r.example',
            'bob@q.example',
            'cat@q.example',
        ])
    })

    it('lists each address once', () => {
        const rows = [
            row('ann@q.example', 'ann', ['g_sales']),
            row('ann@q.example', 'ann', ['g_sales']),
        ]
        expect(addressesLost(rows, new Map(), 'g_sales')).toEqual(['ann@q.example'])
    })
})
