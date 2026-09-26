import { describe, expect, it } from 'vitest'
import {
    domainPanelTarget,
    hasVerifiedDomain,
    setupAddDomainError,
    testMessageRequest,
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
        expect(setupAddDomainError({ status: 503, response: {} })).toBe(
            'Set up email sending first, then come back to this step.'
        )
    })

    it('keeps the default message for any other error', () => {
        expect(setupAddDomainError({ status: 409 })).toBeNull()
        expect(setupAddDomainError(new Error('boom'))).toBeNull()
        expect(setupAddDomainError(null)).toBeNull()
    })
})
