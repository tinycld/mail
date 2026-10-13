import type { VerificationDetails } from '@tinycld/app-generated/mail-api'
import { describe, expect, it } from 'vitest'
import { verificationProblems } from '~/tinycld/mail/settings/verification-problems'

const allPassed = {
    domain: 'acme.com',
    mx_verified: true,
    inbound_domain_verified: true,
    spf_verified: true,
    dkim_verified: true,
    return_path_verified: true,
}

function details(overrides: Partial<VerificationDetails> = {}): VerificationDetails {
    return {
        mx: { ok: true, expected: 'inbound.postmarkapp.com' },
        provider: { ok: true },
        outbound: {
            spf: true,
            dkim: true,
            return_path: true,
            enrolled: 'yes',
            mx_verified: true,
            dkim_host: '20261013._domainkey.acme.com',
            dkim_text_value: 'k=rsa; p=abc',
            return_path_domain: 'pm-bounces.acme.com',
            return_path_cname_value: 'pm.mtasv.net',
        },
        ...overrides,
    }
}

describe('verificationProblems', () => {
    it('reports nothing before the first check', () => {
        expect(verificationProblems({ ...allPassed, mx_verified: false }, null)).toEqual([])
    })

    it('reports nothing when every check passed', () => {
        expect(verificationProblems(allPassed, details())).toEqual([])
    })

    it('names a missing MX record and the value it needs', () => {
        const problems = verificationProblems({ ...allPassed, mx_verified: false }, details())
        expect(problems).toEqual([
            {
                label: 'MX record',
                detail: 'No MX record found for acme.com. Add one with the value 10 inbound.postmarkapp.com.',
            },
        ])
    })

    it('shows an MX that points somewhere else', () => {
        const problems = verificationProblems(
            { ...allPassed, mx_verified: false },
            details({
                mx: { ok: false, expected: 'inbound.postmarkapp.com', actual: ['mx.old-host.com'] },
            })
        )
        expect(problems[0].detail).toBe(
            'acme.com has MX mx.old-host.com, but it must be 10 inbound.postmarkapp.com.'
        )
    })

    it('names the host of each sending record that was not found', () => {
        const problems = verificationProblems(
            { ...allPassed, dkim_verified: false, return_path_verified: false },
            details()
        )
        expect(problems.map(p => p.detail)).toEqual([
            'No matching TXT record found at 20261013._domainkey.acme.com.',
            'No CNAME record at pm-bounces.acme.com points to pm.mtasv.net.',
        ])
    })

    it('says when the provider could not be reached rather than blaming the records', () => {
        const d = details()
        const problems = verificationProblems(allPassed, {
            ...d,
            outbound: { ...d.outbound, enrolled: 'unknown', error: 'timeout' },
        })
        expect(problems).toEqual([
            {
                label: 'Mail provider',
                detail: 'The mail provider could not be reached, so the sending records were not checked. timeout',
            },
        ])
    })
})
