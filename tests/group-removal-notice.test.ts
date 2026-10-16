import { describe, expect, it } from 'vitest'
import { addressesLost, removalNoticeText } from '~/tinycld/mail/settings/group-removal-notice'

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
