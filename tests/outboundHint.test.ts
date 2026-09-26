import type { VerificationDetails } from '@tinycld/app-generated/mail-api'
import { describe, expect, it } from 'vitest'
import { buildOutboundHint } from '~/tinycld/mail/settings/provider'

// FIX 9: the three-valued `enrolled` must actually reach the user.
//
// buildOutboundHint used to return only `details?.outbound?.error`, so the UI
// showed the same raw sentinel text whether the provider REJECTED the domain
// ("no" — the admin's to fix) or the call never completed ("unknown" — not the
// admin's problem, and the row's flags are stale). That is exactly the
// conflation the three-valued type was introduced to fix, undone at the last
// step before the screen.
const details = (outbound: VerificationDetails['outbound']): VerificationDetails =>
    ({ outbound }) as VerificationDetails

describe('buildOutboundHint', () => {
    it('names enrollment as the cause when the provider rejected the domain', () => {
        const hint = buildOutboundHint(
            details({
                spf: false,
                dkim: false,
                return_path: false,
                enrolled: 'no',
                error: 'maildomains: domain not enrolled with the provider: acme.com',
            })
        )

        expect(hint).toMatch(/not enrolled/i)
        expect(hint).not.toMatch(/could not reach/i)
    })

    it('says the check did not run when the provider was unreachable', () => {
        const hint = buildOutboundHint(
            details({
                spf: false,
                dkim: false,
                return_path: false,
                enrolled: 'unknown',
                error: 'socket closed',
            })
        )

        expect(hint).toMatch(/could not reach|not checked/i)
        expect(hint).not.toMatch(/not enrolled with/i)
    })

    // The point of the fix: the two states must not read the same. With the
    // same error text on both, only the enrolled value distinguishes them.
    it('phrases "no" and "unknown" differently even for identical error text', () => {
        const shared = { spf: false, dkim: false, return_path: false, error: 'boom' }

        const no = buildOutboundHint(details({ ...shared, enrolled: 'no' }))
        const unknown = buildOutboundHint(details({ ...shared, enrolled: 'unknown' }))

        expect(no).not.toEqual(unknown)
    })

    it('still carries the provider error detail alongside the cause', () => {
        const hint = buildOutboundHint(
            details({
                spf: false,
                dkim: false,
                return_path: false,
                enrolled: 'no',
                error: 'acme.com was removed',
            })
        )
        expect(hint).toContain('acme.com was removed')
    })

    it('falls back to a neutral label for an enrolled domain', () => {
        expect(
            buildOutboundHint(
                details({ spf: true, dkim: true, return_path: true, enrolled: 'yes' })
            )
        ).toBe('outbound sending')
        expect(buildOutboundHint(null)).toBe('outbound sending')
    })
})
