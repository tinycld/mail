import { describe, expect, it } from 'vitest'
import { filterOwnAddresses, pickReplyFrom } from '~/tinycld/mail/hooks/defaultFromIdentity'
import type { SendableIdentity } from '~/tinycld/mail/hooks/groupSendableIdentities'

const addr = (id: string, email: string) => ({
    id,
    email,
    isPrimary: false,
    domainRank: 1,
    domainVerified: true,
    created: '',
})

const personal: SendableIdentity = {
    mailboxId: 'mb1',
    mailboxDisplayName: 'Nathan',
    addresses: [addr('org', 'nathan@tinycld.org'), addr('com', 'nathan@tinycld.com')],
    defaultAddressId: 'org',
}
const support: SendableIdentity = {
    mailboxId: 'mb2',
    mailboxDisplayName: 'Support',
    addresses: [addr('sup', 'support@tinycld.org'), addr('help', 'help@tinycld.org')],
    defaultAddressId: 'sup',
}
const identities = [personal, support]

describe('pickReplyFrom', () => {
    it('uses the address the message was delivered to', () => {
        expect(
            pickReplyFrom({
                identities,
                deliveredToAddressId: 'com',
                headerAddresses: [],
                threadMailboxId: 'mb1',
            })
        ).toEqual({ mailboxId: 'mb1', addressId: 'com' })
    })

    it('falls back to a header match for mail stored before delivered_to existed', () => {
        expect(
            pickReplyFrom({
                identities,
                deliveredToAddressId: null,
                headerAddresses: ['"Help" <HELP@tinycld.org>'],
                threadMailboxId: 'mb1',
            })
        ).toEqual({ mailboxId: 'mb2', addressId: 'help' })
    })

    it("falls back to the thread mailbox's default", () => {
        expect(
            pickReplyFrom({
                identities,
                deliveredToAddressId: 'gone',
                headerAddresses: ['someone@else.example'],
                threadMailboxId: 'mb2',
            })
        ).toEqual({ mailboxId: 'mb2', addressId: null })
    })

    it('returns an empty selection with no identities', () => {
        expect(
            pickReplyFrom({
                identities: [],
                deliveredToAddressId: null,
                headerAddresses: [],
                threadMailboxId: 'x',
            })
        ).toEqual({ mailboxId: '', addressId: null })
    })
})

describe('filterOwnAddresses', () => {
    it('removes every address of the identity, any case', () => {
        const recipients = [
            { name: 'Me', email: 'Nathan@TinyCld.com' },
            { name: 'Bo', email: 'bo@else.example' },
        ]
        expect(filterOwnAddresses({ identity: personal, recipients })).toEqual([
            { name: 'Bo', email: 'bo@else.example' },
        ])
    })
})
