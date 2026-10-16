import { describe, expect, it } from 'vitest'
import { composeSendBlocker, resolveComposeFrom } from '~/tinycld/mail/hooks/composeFrom'
import type { SendableIdentity } from '~/tinycld/mail/hooks/groupSendableIdentities'

const address = (id: string, email: string, domainVerified = true) => ({
    id,
    email,
    isPrimary: false,
    domainRank: 1,
    domainVerified,
    created: '',
})

const personal: SendableIdentity = {
    mailboxId: 'mb_me',
    mailboxDisplayName: 'Nathan',
    isPersonal: true,
    addresses: [address('org', 'nathan@tinycld.org'), address('com', 'nathan@tinycld.com')],
    defaultAddressId: 'org',
}
const support: SendableIdentity = {
    mailboxId: 'mb_support',
    mailboxDisplayName: 'Support',
    isPersonal: false,
    addresses: [
        address('sup', 'support@tinycld.org'),
        address('pend', 'help@pending.example', false),
    ],
    defaultAddressId: 'sup',
}
const identities = [personal, support]

describe('resolveComposeFrom', () => {
    it('uses the first identity with nothing picked and no fallback', () => {
        const from = resolveComposeFrom(identities, '', null, '')
        expect(from?.identity.mailboxId).toBe('mb_me')
        expect(from?.address.id).toBe('org')
    })

    it('uses the fallback mailbox when nothing is picked', () => {
        expect(resolveComposeFrom(identities, '', null, 'mb_support')?.address.id).toBe('sup')
    })

    it('uses the picked shared mailbox and address', () => {
        const from = resolveComposeFrom(identities, 'mb_support', 'pend', 'mb_me')
        expect(from?.identity.mailboxId).toBe('mb_support')
        expect(from?.address.id).toBe('pend')
    })

    it("falls back to the mailbox's default for a stale address id", () => {
        expect(resolveComposeFrom(identities, 'mb_support', 'gone', '')?.address.id).toBe('sup')
    })

    it('ignores an address of another mailbox', () => {
        expect(resolveComposeFrom(identities, 'mb_support', 'com', '')?.address.id).toBe('sup')
    })

    it('resolves nothing while the wanted mailbox is not loaded, instead of another mailbox', () => {
        expect(resolveComposeFrom([personal], 'mb_support', 'sup', 'mb_me')).toBeNull()
        expect(resolveComposeFrom([], '', null, 'mb_me')).toBeNull()
    })
})

describe('composeSendBlocker', () => {
    const ready = { mailboxId: 'mb_me', blocker: null, message: null }

    it('warns when the chosen address is on an unverified domain', () => {
        const from = resolveComposeFrom(identities, 'mb_support', 'pend', '')
        const result = composeSendBlocker(from, ready)
        expect(result.blocker).toBe('domain-unverified')
        expect(result.message).toContain('pending.example')
    })

    it('does not warn about the personal domain when the chosen address is verified', () => {
        const from = resolveComposeFrom(identities, 'mb_support', 'sup', '')
        const personalUnverified = {
            mailboxId: 'mb_me',
            blocker: 'domain-unverified' as const,
            message: 'The domain "tinycld.org" is not verified.',
        }
        expect(composeSendBlocker(from, personalUnverified)).toEqual({
            blocker: null,
            message: null,
        })
    })

    it('keeps the no-mailbox and no-domain blockers when there is no identity', () => {
        const noMailbox = {
            mailboxId: null,
            blocker: 'no-mailbox' as const,
            message: 'No mailbox found.',
        }
        expect(composeSendBlocker(null, noMailbox)).toEqual({
            blocker: 'no-mailbox',
            message: 'No mailbox found.',
        })
        const noDomain = {
            mailboxId: 'mb_me',
            blocker: 'no-domain' as const,
            message: 'No domain.',
        }
        expect(composeSendBlocker(null, noDomain).blocker).toBe('no-domain')
    })

    it('does not report a personal-domain warning when no address is resolved yet', () => {
        const unverified = {
            mailboxId: 'mb_me',
            blocker: 'domain-unverified' as const,
            message: 'x',
        }
        expect(composeSendBlocker(null, unverified)).toEqual({ blocker: null, message: null })
    })
})
