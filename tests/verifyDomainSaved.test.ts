import type { VerifyDomainResponse } from '@tinycld/app-generated/mail-api'
import { describe, expect, it } from 'vitest'
import { assertVerifySaved } from '~/tinycld/mail/settings/verify-domain'

function response(overrides: Partial<VerifyDomainResponse>): VerifyDomainResponse {
    return {
        id: 'dom1',
        verified: false,
        mx_verified: false,
        inbound_domain_verified: false,
        spf_verified: false,
        dkim_verified: false,
        return_path_verified: false,
        last_checked_at: '2026-08-06T00:00:00Z',
        verification_details: null,
        saved: true,
        ...overrides,
    }
}

describe('assertVerifySaved', () => {
    it('passes a saved response through unchanged', () => {
        const res = response({ verified: true })
        expect(assertVerifySaved(res)).toBe(res)
    })

    it('throws the server save_error when persistence failed', () => {
        // The endpoint returns HTTP 200 with saved:false in this case — the
        // bug was the client discarding the body and reporting success.
        expect(() =>
            assertVerifySaved(response({ saved: false, save_error: 'disk full' }))
        ).toThrow('disk full')
    })

    it('throws a generic message when save_error is absent', () => {
        expect(() => assertVerifySaved(response({ saved: false }))).toThrow(/could not be saved/)
    })
})
