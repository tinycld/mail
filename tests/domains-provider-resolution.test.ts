import { describe, expect, it } from 'vitest'
import { resolveProvider } from '~/tinycld/mail/settings/provider'

// The Domains panel stays visible on a deployment whose operator owns mail, and
// it prints provider-specific DNS: the MX target to publish, and whether
// Postmark webhook URLs apply. Getting the provider wrong there is not cosmetic
// — it is DNS a user will act on.
describe('resolveProvider', () => {
    const details = (name?: string) => ({ provider_name: name }) as never

    it("prefers the server's answer over the stored row", () => {
        // The operator runs SMTP; this deployment has no mail.provider row.
        expect(resolveProvider(details('smtp'), undefined)).toBe('smtp')
    })

    it('uses the stored row when the server has not reported one', () => {
        // Self-hosted, no domain verified yet, so no server answer exists.
        expect(resolveProvider(null, 'smtp')).toBe('smtp')
        expect(resolveProvider(details(undefined), 'smtp')).toBe('smtp')
    })

    // The regression: with no row and no server answer the panel defaulted to
    // Postmark, printing inbound.postmarkapp.com as the MX target on a
    // deployment that sends over SMTP.
    it('does not invent Postmark when the server says otherwise', () => {
        expect(resolveProvider(details('smtp'), 'postmark')).toBe('smtp')
    })

    it('falls back to postmark only when nothing says otherwise', () => {
        expect(resolveProvider(null, undefined)).toBe('postmark')
    })

    // "none" means no provider is configured at all; it is not a DNS variant,
    // so the panel must not render it as one.
    it('treats an unconfigured provider as the postmark default', () => {
        expect(resolveProvider(details('none'), undefined)).toBe('postmark')
    })
})
