import { describe, expect, it } from 'vitest'
import {
    composeFromRowView,
    composeSendBlocker,
    composeSendRequest,
    draftSaveRequest,
    resolveComposeFrom,
    resolveComposeState,
} from '~/tinycld/mail/hooks/composeFrom'
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

describe('resolveComposeState with an open draft', () => {
    it("offers only the draft's mailbox, still with all its addresses", () => {
        const { choices, from } = resolveComposeState({
            identities,
            storeMailboxId: 'mb_support',
            storeAddressId: null,
            fallbackMailboxId: 'mb_support',
            lockedMailboxId: 'mb_support',
        })
        expect(choices.map(i => i.mailboxId)).toEqual(['mb_support'])
        expect(choices[0].addresses.map(a => a.id)).toEqual(['sup', 'pend'])
        expect(from?.identity.mailboxId).toBe('mb_support')
    })

    it("keeps the draft's mailbox even if the store names another one", () => {
        const { from } = resolveComposeState({
            identities,
            storeMailboxId: 'mb_me',
            storeAddressId: 'org',
            fallbackMailboxId: 'mb_support',
            lockedMailboxId: 'mb_support',
        })
        expect(from?.identity.mailboxId).toBe('mb_support')
        expect(from?.address.id).toBe('sup')
    })

    it('offers every mailbox without a draft', () => {
        const { choices } = resolveComposeState({
            identities,
            storeMailboxId: '',
            storeAddressId: null,
            fallbackMailboxId: 'mb_me',
            lockedMailboxId: null,
        })
        expect(choices.map(i => i.mailboxId)).toEqual(['mb_me', 'mb_support'])
    })
})

describe('resolveComposeState with no From', () => {
    it("says the draft's mailbox can no longer send once identities have loaded", () => {
        const { from, missingFromMessage } = resolveComposeState({
            identities,
            storeMailboxId: '',
            storeAddressId: null,
            fallbackMailboxId: 'mb_gone',
            lockedMailboxId: 'mb_gone',
        })
        expect(from).toBeNull()
        expect(missingFromMessage).toBe("You can no longer send from this draft's mailbox")
    })

    it('says the address has not loaded while there are no identities yet', () => {
        const { missingFromMessage } = resolveComposeState({
            identities: [],
            storeMailboxId: '',
            storeAddressId: null,
            fallbackMailboxId: 'mb_gone',
            lockedMailboxId: 'mb_gone',
        })
        expect(missingFromMessage).toBe('No address to save it from has loaded yet')
    })

    it('has no message when a From resolves', () => {
        const { missingFromMessage } = resolveComposeState({
            identities,
            storeMailboxId: '',
            storeAddressId: null,
            fallbackMailboxId: 'mb_me',
            lockedMailboxId: null,
        })
        expect(missingFromMessage).toBeNull()
    })
})

describe('composeFromRowView', () => {
    it('shows the resolved From and offers the picker when there is a choice', () => {
        const from = resolveComposeFrom(identities, '', null, 'mb_me')
        expect(composeFromRowView(identities, from)).toEqual({
            isVisible: true,
            label: 'Nathan <nathan@tinycld.org>',
            hasChoices: true,
            selectedAddressId: 'org',
        })
    })

    it('hides the picker when the only mailbox has one address', () => {
        const single = [{ ...support, addresses: [support.addresses[0]] }]
        const from = resolveComposeFrom(single, '', null, 'mb_support')
        expect(composeFromRowView(single, from).hasChoices).toBe(false)
    })

    // The personal mailbox has no sendable address, so nothing resolves; the
    // user must still be able to pick the shared mailbox.
    it('offers the picker when no From resolves but other mailboxes can send', () => {
        expect(composeFromRowView([support], null)).toEqual({
            isVisible: true,
            label: 'Choose an address to send from',
            hasChoices: true,
            selectedAddressId: '',
        })
    })

    it('hides the row when there is nothing to send from', () => {
        expect(composeFromRowView([], null).isVisible).toBe(false)
    })
})

describe('draftSaveRequest', () => {
    it('saves a shared-mailbox draft into its own mailbox', () => {
        const { from } = resolveComposeState({
            identities,
            storeMailboxId: '',
            storeAddressId: null,
            fallbackMailboxId: 'mb_support',
            lockedMailboxId: 'mb_support',
        })
        if (!from) throw new Error('expected a From')
        const request = draftSaveRequest(from, 'd1', {
            subject: 'Plan',
            html_body: '<p>x</p>',
            text_body: 'x',
        })
        expect(request.mailbox_id).toBe('mb_support')
        expect(request.from_address_id).toBe('sup')
        expect(request.message_id).toBe('d1')
    })

    it('creates a new draft when none is open', () => {
        const from = resolveComposeFrom(identities, '', null, '')
        if (!from) throw new Error('expected a From')
        expect(draftSaveRequest(from, null, { subject: '' }).message_id).toBeUndefined()
    })
})

describe('draftSaveRequest with removed saved attachments', () => {
    it('asks the server to drop the removed files from the open draft', () => {
        const from = resolveComposeFrom(identities, '', null, '')
        if (!from) throw new Error('expected a From')
        const request = draftSaveRequest(from, 'd1', { subject: 's' }, ['hippo_ab12cd34ef.jpg'])
        expect(request.remove_attachments).toEqual(['hippo_ab12cd34ef.jpg'])
    })

    it('sends no removals for a new draft', () => {
        const from = resolveComposeFrom(identities, '', null, '')
        if (!from) throw new Error('expected a From')
        const request = draftSaveRequest(from, null, { subject: 's' }, ['hippo_ab12cd34ef.jpg'])
        expect(request.remove_attachments).toBeUndefined()
    })
})

describe('composeSendRequest', () => {
    const fields = {
        to: [{ name: '', email: 'a@b.example' }],
        subject: 's',
        html_body: '',
        text_body: '',
    }

    it('names the resumed draft and the saved files compose kept', () => {
        const { from } = resolveComposeState({
            identities,
            storeMailboxId: '',
            storeAddressId: null,
            fallbackMailboxId: 'mb_support',
            lockedMailboxId: 'mb_support',
        })
        if (!from) throw new Error('expected a From')
        const request = composeSendRequest(from, 'd1', ['keep_ab12cd34ef.txt'], fields)
        expect(request.mailbox_id).toBe('mb_support')
        expect(request.from_address_id).toBe('sup')
        expect(request.draft_id).toBe('d1')
        expect(request.draft_attachments).toEqual(['keep_ab12cd34ef.txt'])
    })

    it('names no saved files when compose showed none, so a deleted draft still sends', () => {
        const from = resolveComposeFrom(identities, '', null, '')
        if (!from) throw new Error('expected a From')
        const request = composeSendRequest(from, 'd1', [], fields)
        expect(request.draft_id).toBe('d1')
        expect(request.draft_attachments).toBeUndefined()
    })

    it('names no draft for a new message', () => {
        const from = resolveComposeFrom(identities, '', null, '')
        if (!from) throw new Error('expected a From')
        const request = composeSendRequest(from, null, [], fields)
        expect(request.draft_id).toBeUndefined()
        expect(request.draft_attachments).toBeUndefined()
    })
})
