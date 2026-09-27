import { describe, expect, it } from 'vitest'
import {
    domainPanelTarget,
    hasVerifiedDomain,
    setupAddDomainError,
    testMessageLabel,
    testMessageRequest,
    testMessageState,
    verifiedDomainOptions,
} from '~/tinycld/mail/setup/setup-logic'

const domains = [
    { id: 'd1', domain: 'pending.test', verified: false },
    { id: 'd2', domain: 'ready.test', verified: true },
]

describe('hasVerifiedDomain', () => {
    it('is false with no rows', () => {
        expect(hasVerifiedDomain([])).toBe(false)
    })

    it('is false when no row is verified', () => {
        expect(hasVerifiedDomain([domains[0]])).toBe(false)
    })

    it('is true when one row is verified', () => {
        expect(hasVerifiedDomain(domains)).toBe(true)
    })
})

describe('verifiedDomainOptions', () => {
    it('offers only verified domains, keyed by record id', () => {
        expect(verifiedDomainOptions(domains)).toEqual([{ label: 'ready.test', value: 'd2' }])
    })
})

describe('domainPanelTarget', () => {
    const rows = [
        { id: 'a', verified: false },
        { id: 'b', verified: true },
        { id: 'c', verified: false },
        { id: 'd', verified: true },
    ]

    it('picks the domain added in this visit, even once verified', () => {
        expect(domainPanelTarget(rows, 'b')?.id).toBe('b')
    })

    it('is undefined while the added domain has not synced yet', () => {
        expect(domainPanelTarget(rows, 'new')).toBeUndefined()
    })

    it('otherwise picks the newest unverified domain', () => {
        expect(domainPanelTarget(rows, undefined)?.id).toBe('c')
    })

    it('is undefined when every domain is verified', () => {
        expect(domainPanelTarget([{ id: 'b', verified: true }], undefined)).toBeUndefined()
    })
})

describe('testMessageRequest', () => {
    it('sends from the mailbox to the address, named for the workspace', () => {
        const req = testMessageRequest({
            mailboxId: 'mb1',
            to: 'owner@example.com',
            workspaceName: 'Acme',
        })
        expect(req.mailbox_id).toBe('mb1')
        expect(req.to).toEqual([{ email: 'owner@example.com', name: '' }])
        expect(req.subject).toBe('Test message from Acme')
        expect(req.text_body).toContain('Acme')
        expect(req.html_body).toContain('Acme')
    })

    it('escapes the workspace name in the HTML body', () => {
        const req = testMessageRequest({ mailboxId: 'mb1', to: 'a@b.test', workspaceName: 'A<b>' })
        expect(req.html_body).toContain('A&lt;b&gt;')
        expect(req.html_body).not.toContain('<b>')
    })

    it('falls back when the workspace has no name', () => {
        const req = testMessageRequest({ mailboxId: 'mb1', to: 'a@b.test', workspaceName: '  ' })
        expect(req.subject).toBe('Test message from your workspace')
    })
})

describe('setupAddDomainError', () => {
    it('sends a 503 to the Email sending step', () => {
        expect(setupAddDomainError({ status: 503, response: {} }, false)).toBe(
            'Set up email sending first, then come back to this step.'
        )
    })

    it('sends a 503 to the administrator when mail settings are managed', () => {
        expect(setupAddDomainError({ status: 503, response: {} }, true)).toBe(
            'Mail domains are not available yet. Ask your administrator.'
        )
    })

    it('keeps the default message for any other error', () => {
        expect(setupAddDomainError({ status: 409 }, false)).toBeNull()
        expect(setupAddDomainError(new Error('boom'), true)).toBeNull()
        expect(setupAddDomainError(null, false)).toBeNull()
    })
})

describe('testMessageState', () => {
    const row = (
        o: Partial<{
            delivery_status: string
            delivered_at: string
            bounce_class: string
            bounce_reason: string
        }>
    ) => ({
        delivery_status: 'sent',
        delivered_at: '',
        bounce_class: '',
        bounce_reason: '',
        ...o,
    })
    it('is none with no message', () =>
        expect(testMessageState(undefined)).toEqual({ kind: 'none' }))
    it('is sending while the provider has not answered', () =>
        expect(testMessageState(row({ delivery_status: 'sending' }))).toEqual({ kind: 'sending' }))
    it('is sent when accepted but not delivered', () =>
        expect(testMessageState(row({}))).toEqual({ kind: 'sent' }))
    it('is delivered once delivered_at is set', () =>
        expect(testMessageState(row({ delivered_at: '2026-09-26 10:00:00Z' }))).toEqual({
            kind: 'delivered',
        }))
    it('is bounced with class and reason', () =>
        expect(
            testMessageState(
                row({
                    delivery_status: 'bounced',
                    bounce_class: 'hard',
                    bounce_reason: 'No such user',
                })
            )
        ).toEqual({ kind: 'bounced', bounceClass: 'hard', reason: 'No such user' }))
    it('treats a spam complaint as bounced', () =>
        expect(
            testMessageState(row({ delivery_status: 'spam_complaint', bounce_class: 'complaint' }))
                .kind
        ).toBe('bounced'))
})

describe('testMessageLabel', () => {
    it('names each state', () => {
        expect(testMessageLabel({ kind: 'sent' })).toBe('Sent. Waiting for delivery…')
        expect(testMessageLabel({ kind: 'delivered' })).toBe('Delivered')
        expect(testMessageLabel({ kind: 'bounced', bounceClass: 'hard', reason: 'x' })).toBe(
            'Bounced (hard): x'
        )
        expect(testMessageLabel({ kind: 'bounced', bounceClass: '', reason: 'x' })).toBe(
            'Bounced: x'
        )
    })

    it('drops the colon when a bounce carries no reason', () => {
        expect(testMessageLabel({ kind: 'bounced', bounceClass: 'hard', reason: '' })).toBe(
            'Bounced (hard)'
        )
    })
})
