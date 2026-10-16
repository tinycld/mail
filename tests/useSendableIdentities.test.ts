import { describe, expect, it } from 'vitest'
import {
    groupSendableIdentities,
    type SendableIdentityRow,
} from '~/tinycld/mail/hooks/groupSendableIdentities'
import type {
    MailAddresses,
    MailDomains,
    MailIdentities,
    MailMailboxes,
} from '~/tinycld/mail/types'

function mb(overrides: Partial<MailMailboxes>): MailMailboxes {
    return {
        id: 'mb1',
        address: 'nathan',
        domain: 'org',
        display_name: 'Nathan',
        name: 'Nathan',
        type: 'personal',
        created: '2024-01-01T00:00:00Z',
        updated: '',
        ...overrides,
    } as MailMailboxes
}

function dn(id: string, domain: string, rank: number, verified = true): MailDomains {
    return { id, domain, rank, verified, coverage: 'everyone' } as MailDomains
}

function ad(
    id: string,
    mailbox: string,
    domain: string,
    local: string,
    extra: Partial<MailAddresses> = {}
): MailAddresses {
    return {
        id,
        mailbox,
        domain,
        local_part: local,
        source: 'manual',
        is_primary: false,
        created: '2024-01-01T00:00:00Z',
        updated: '',
        ...extra,
    } as MailAddresses
}

const org = dn('org', 'tinycld.org', 1)
const com = dn('com', 'tinycld.com', 2)

describe('groupSendableIdentities', () => {
    it('lists every address of a mailbox, default first', () => {
        const mailbox = mb({})
        const rows: SendableIdentityRow[] = [
            {
                mailbox,
                domain: com,
                address: ad('a_com', 'mb1', 'com', 'nathan', { source: 'rule' }),
            },
            {
                mailbox,
                domain: org,
                address: ad('a_org', 'mb1', 'org', 'nathan', { is_primary: true }),
            },
        ]
        const [identity] = groupSendableIdentities(rows)
        expect(identity.defaultAddressId).toBe('a_org')
        expect(identity.addresses.map(a => a.email)).toEqual([
            'nathan@tinycld.org',
            'nathan@tinycld.com',
        ])
    })

    it("uses the identity's default_from when it is one of the addresses", () => {
        const mailbox = mb({})
        const identity = {
            id: 'i1',
            user: 'u1',
            inbox: 'mb1',
            email_name: 'nathan',
            default_from: 'a_com',
        } as MailIdentities
        const rows: SendableIdentityRow[] = [
            {
                mailbox,
                domain: org,
                address: ad('a_org', 'mb1', 'org', 'nathan', { is_primary: true }),
                identity,
            },
            { mailbox, domain: com, address: ad('a_com', 'mb1', 'com', 'nathan'), identity },
        ]
        expect(groupSendableIdentities(rows)[0].defaultAddressId).toBe('a_com')
    })

    it('prefers a verified domain over a better-ranked unverified one', () => {
        const mailbox = mb({})
        const pending = dn('pending', 'pending.example', 0, false)
        const rows: SendableIdentityRow[] = [
            { mailbox, domain: pending, address: ad('a_p', 'mb1', 'pending', 'nathan') },
            { mailbox, domain: com, address: ad('a_com', 'mb1', 'com', 'nathan') },
        ]
        expect(groupSendableIdentities(rows)[0].defaultAddressId).toBe('a_com')
    })

    it('orders personal first, then shared by created', () => {
        const rows: SendableIdentityRow[] = [
            {
                mailbox: mb({ id: 'sh2', type: 'shared', created: '2024-03-01T00:00:00Z' }),
                domain: org,
                address: ad('x2', 'sh2', 'org', 'ops'),
            },
            {
                mailbox: mb({ id: 'sh1', type: 'shared', created: '2024-02-01T00:00:00Z' }),
                domain: org,
                address: ad('x1', 'sh1', 'org', 'support'),
            },
            { mailbox: mb({}), domain: org, address: ad('a_org', 'mb1', 'org', 'nathan') },
        ]
        expect(groupSendableIdentities(rows).map(i => i.mailboxId)).toEqual(['mb1', 'sh1', 'sh2'])
    })

    it('collapses the duplicate rows a left join can produce', () => {
        const mailbox = mb({})
        const address = ad('a_org', 'mb1', 'org', 'nathan')
        const rows: SendableIdentityRow[] = [
            { mailbox, domain: org, address },
            { mailbox, domain: org, address },
        ]
        expect(groupSendableIdentities(rows)[0].addresses).toHaveLength(1)
    })

    it('returns nothing for no rows', () => {
        expect(groupSendableIdentities([])).toEqual([])
    })
})
