import { describe, expect, it } from 'vitest'
import {
    groupSendableIdentities,
    type SendableIdentityRow,
} from '~/tinycld/mail/hooks/flattenSendableIdentities'
import type { MailDomains, MailMailboxAliases, MailMailboxes } from '~/tinycld/mail/types'

// These cover the pure grouping of useSendableIdentities' joined rows. The
// join semantics themselves (membership predicate, domain inner join, alias
// left join) are covered against real collections in
// tests/useMailboxHooks.mount.test.tsx.

function mb(overrides: Partial<MailMailboxes>): MailMailboxes {
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

function dn(id: string, domain: string): MailDomains {
    return {
        id,
        domain,
        verified: true,
        mx_verified: true,
        inbound_domain_verified: true,
        spf_verified: true,
        dkim_verified: true,
        return_path_verified: true,
        last_checked_at: '',
        verification_details: null,
        webhook_secret: '',
        created: '',
        updated: '',
    }
}

function al(id: string, mailbox: string, address: string): MailMailboxAliases {
    return {
        id,
        mailbox,
        address,
        created: '',
        updated: '',
    }
}

describe('groupSendableIdentities', () => {
    it('includes mailbox primary + its aliases', () => {
        const mailbox = mb({ id: 'mb1', address: 'alice', domain: 'd1' })
        const rows: SendableIdentityRow[] = [
            { mailbox, domain: dn('d1', 'acme.com'), alias: al('a1', 'mb1', 'alice.smith') },
        ]
        const got = groupSendableIdentities(rows)
        expect(got).toHaveLength(1)
        expect(got[0].mailboxId).toBe('mb1')
        expect(got[0].primaryAddress).toBe('alice@acme.com')
        expect(got[0].aliases).toEqual([{ id: 'a1', address: 'alice.smith@acme.com' }])
    })

    it('a mailbox with no aliases still yields an identity (left-join row)', () => {
        const rows: SendableIdentityRow[] = [
            { mailbox: mb({ id: 'mb1' }), domain: dn('d1', 'acme.com') },
        ]
        const got = groupSendableIdentities(rows)
        expect(got).toHaveLength(1)
        expect(got[0].aliases).toEqual([])
    })

    it('collapses multiple alias rows into one identity per mailbox', () => {
        const shared = mb({
            id: 'mb2',
            address: 'support',
            domain: 'd1',
            type: 'shared',
            display_name: 'Support',
        })
        const domain = dn('d1', 'acme.com')
        const rows: SendableIdentityRow[] = [
            { mailbox: shared, domain, alias: al('a1', 'mb2', 'help') },
            { mailbox: shared, domain, alias: al('a2', 'mb2', 'sos') },
        ]
        const got = groupSendableIdentities(rows)
        expect(got).toHaveLength(1)
        expect(got[0].aliases).toEqual([
            { id: 'a1', address: 'help@acme.com' },
            { id: 'a2', address: 'sos@acme.com' },
        ])
    })

    it('orders personal first, then shared by created ascending', () => {
        const domain = dn('d1', 'acme.com')
        const rows: SendableIdentityRow[] = [
            {
                mailbox: mb({
                    id: 'mb_late',
                    type: 'shared',
                    address: 'late',
                    created: '2024-06-01T00:00:00Z',
                }),
                domain,
            },
            {
                mailbox: mb({
                    id: 'mb_early',
                    type: 'shared',
                    address: 'early',
                    created: '2024-01-02T00:00:00Z',
                }),
                domain,
            },
            { mailbox: mb({ id: 'mb_me', type: 'personal', address: 'alice' }), domain },
        ]
        const got = groupSendableIdentities(rows)
        expect(got.map(i => i.mailboxId)).toEqual(['mb_me', 'mb_early', 'mb_late'])
    })

    it('falls back to address when display_name is empty', () => {
        const rows: SendableIdentityRow[] = [
            {
                mailbox: mb({ id: 'mb1', address: 'alice', display_name: '' }),
                domain: dn('d1', 'acme.com'),
            },
        ]
        expect(groupSendableIdentities(rows)[0].mailboxDisplayName).toBe('alice')
    })

    it('returns empty list for no rows', () => {
        expect(groupSendableIdentities([])).toEqual([])
    })
})
