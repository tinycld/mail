import { describe, expect, it } from 'vitest'
import { defaultAddressFor, newMailboxRecords } from '~/tinycld/mail/settings/mailbox-records'

const data = { address: 'alice', domain: 'd1', display_name: 'Alice' }

describe('newMailboxRecords', () => {
    it('writes a personal mailbox with the creator as owner', () => {
        const { mailbox, member } = newMailboxRecords({
            data,
            type: 'personal',
            userId: 'u1',
            mailboxId: 'mb1',
            memberId: 'm1',
        })
        expect(mailbox).toEqual({
            id: 'mb1',
            address: 'alice',
            domain: 'd1',
            display_name: 'Alice',
            name: 'Alice',
            type: 'personal',
        })
        expect(member).toEqual({ id: 'm1', mailbox: 'mb1', user: 'u1', role: 'owner' })
    })

    it('writes a shared mailbox when asked', () => {
        const { mailbox } = newMailboxRecords({
            data,
            type: 'shared',
            userId: 'u1',
            mailboxId: 'mb1',
            memberId: 'm1',
        })
        expect(mailbox.type).toBe('shared')
    })
})

describe('defaultAddressFor', () => {
    it('lowercases and drops characters the address rule refuses', () => {
        expect(defaultAddressFor(' Alice.Smith+x ')).toBe('alice.smithx')
    })
})
