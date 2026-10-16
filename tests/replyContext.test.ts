import { describe, expect, it } from 'vitest'
import type { SendableIdentity } from '~/tinycld/mail/hooks/groupSendableIdentities'
import { buildReplyContext } from '~/tinycld/mail/hooks/replyContext'

const personal: SendableIdentity = {
    mailboxId: 'mb1',
    mailboxDisplayName: 'Nathan',
    addresses: [
        {
            id: 'org',
            email: 'nathan@tinycld.org',
            isPrimary: true,
            domainRank: 1,
            domainVerified: true,
            created: '',
        },
        {
            id: 'com',
            email: 'nathan@tinycld.com',
            isPrimary: false,
            domainRank: 2,
            domainVerified: true,
            created: '',
        },
    ],
    defaultAddressId: 'org',
}
const message = {
    id: 'm1',
    subject: 'Plan',
    sender_name: 'Bo',
    sender_email: 'bo@else.example',
    recipients_to: [{ name: 'Nathan', email: 'nathan@tinycld.com' }],
    recipients_cc: [{ name: 'Cy', email: 'cy@else.example' }],
    delivered_to: 'com',
}

describe('buildReplyContext', () => {
    it('replies to the sender from the address the message arrived on', () => {
        const ctx = buildReplyContext({
            kind: 'reply',
            message,
            threadId: 't1',
            threadMailboxId: 'mb1',
            identities: [personal],
        })
        expect(ctx.to).toEqual([{ name: 'Bo', email: 'bo@else.example' }])
        expect(ctx.from).toEqual({ mailboxId: 'mb1', addressId: 'com' })
        expect(ctx.subject).toBe('Plan')
        expect(ctx.mailboxId).toBe('mb1')
    })

    it('reply-all drops every one of my addresses', () => {
        const ctx = buildReplyContext({
            kind: 'replyAll',
            message,
            threadId: 't1',
            threadMailboxId: 'mb1',
            identities: [personal],
        })
        expect(ctx.to.map(r => r.email)).toEqual(['bo@else.example', 'cy@else.example'])
    })

    it('forward starts with no recipients and a Fwd subject', () => {
        const ctx = buildReplyContext({
            kind: 'forward',
            message,
            threadId: 't1',
            threadMailboxId: 'mb1',
            identities: [personal],
        })
        expect(ctx.to).toEqual([])
        expect(ctx.subject).toBe('Fwd: Plan')
    })
})
