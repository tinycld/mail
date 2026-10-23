import { describe, expect, it } from 'vitest'
import { defaultAddressFor, newMailboxRecords } from '~/tinycld/mail/settings/mailbox-records'

function counter() {
    let n = 0
    return () => `id${++n}`
}

const base = { address: 'alice', displayName: 'Alice', userId: 'u1' }

describe('newMailboxRecords', () => {
    it('writes a personal mailbox with the creator as owner', () => {
        const { mailbox, member, addresses } = newMailboxRecords({
            ...base,
            domains: ['d1'],
            type: 'personal',
            newId: counter(),
        })
        expect(mailbox).toEqual({
            id: 'id1',
            address: 'alice',
            domain: 'd1',
            display_name: 'Alice',
            name: 'Alice',
            type: 'personal',
        })
        expect(member).toEqual({
            id: 'id2',
            mailbox: 'id1',
            user: 'u1',
            group: '',
            role: 'owner',
        })
        expect(addresses).toEqual([])
    })

    it('writes a shared mailbox when asked', () => {
        const { mailbox } = newMailboxRecords({
            ...base,
            domains: ['d1'],
            type: 'shared',
            newId: counter(),
        })
        expect(mailbox.type).toBe('shared')
    })

    it('uses the first domain as the main address and adds the others', () => {
        const { mailbox, addresses } = newMailboxRecords({
            ...base,
            domains: ['d1', 'd2', 'd3'],
            type: 'shared',
            newId: counter(),
        })
        expect(mailbox.domain).toBe('d1')
        expect(addresses.map(a => [a.local_part, a.domain, a.mailbox, a.is_primary])).toEqual([
            ['alice', 'd2', 'id1', false],
            ['alice', 'd3', 'id1', false],
        ])
        expect(addresses.every(a => a.source === 'manual')).toBe(true)
    })
})

describe('defaultAddressFor', () => {
    it('lowercases and drops characters the address rule refuses', () => {
        expect(defaultAddressFor(' Alice.Smith+x ')).toBe('alice.smithx')
    })
})
