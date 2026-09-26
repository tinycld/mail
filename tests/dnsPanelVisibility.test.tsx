// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import type { VerificationDetails } from '@tinycld/app-generated/mail-api'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it } from 'vitest'
import { DomainVerificationPanel } from '~/tinycld/mail/settings/provider'

// No vitest globals in this workspace, so testing-library's automatic
// afterEach cleanup never registers.
afterEach(cleanup)

// Each DNS row owns a copy-to-clipboard useMutation, so the tree needs a
// QueryClient. Nothing here actually mutates; this is mount scaffolding.
const withQueryClient = (ui: ReactNode) => (
    <QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>
)

// FIX 7: the DNS panel must not vanish while records are still unpublished.
//
// It was gated on `!domain.verified`, but `verified` is deliberately
// inbound-only — DKIM and return-path are advisory and excluded. So as soon as
// MX and the inbound domain passed, the badge went green and the whole panel
// DISAPPEARED, taking the host, value and copy button with it, while the DKIM
// and Return-Path rows right above it were still red. The admin was shown a
// problem and simultaneously denied the only UI that could fix it.
//
// This mounts the real component, so it pins the WIRING, not just the helper:
// a correct hasUnpublishedDnsRecords that nothing calls would still ship the
// bug.
const outbound = {
    spf: false,
    dkim: false,
    return_path: false,
    enrolled: 'yes',
    dkim_host: 'sel._domainkey.acme.com',
    dkim_text_value: 'k=rsa;p=PUBLISHME',
    return_path_domain: 'pm-bounces.acme.com',
    return_path_cname_value: 'pm.mtasv.net',
}

const verifiedDomainWithUnpublishedDkim = {
    id: 'd1',
    domain: 'acme.com',
    // Green badge: both inbound legs passed.
    verified: true,
    mx_verified: true,
    inbound_domain_verified: true,
    // ...but the advisory records are not published yet.
    spf_verified: false,
    dkim_verified: false,
    return_path_verified: false,
    last_checked_at: '2026-09-16T00:00:00Z',
    verification_details: {
        provider_name: 'postmark',
        outbound,
    } as unknown as VerificationDetails,
}

describe('DnsRecordsPanel visibility', () => {
    it('keeps showing the records while a verified domain still has some unpublished', () => {
        render(
            withQueryClient(
                <DomainVerificationPanel
                    domain={verifiedDomainWithUnpublishedDkim}
                    provider="postmark"
                />
            )
        )

        expect(screen.getByText('DNS records to publish')).toBeTruthy()
        expect(screen.getByText('sel._domainkey.acme.com')).toBeTruthy()
        expect(screen.getByText('k=rsa;p=PUBLISHME')).toBeTruthy()
    })

    it('hides the panel once every record is published', () => {
        render(
            withQueryClient(
                <DomainVerificationPanel
                    domain={{
                        ...verifiedDomainWithUnpublishedDkim,
                        spf_verified: true,
                        dkim_verified: true,
                        return_path_verified: true,
                        verification_details: {
                            provider_name: 'postmark',
                            outbound: {
                                ...outbound,
                                spf: true,
                                dkim: true,
                                return_path: true,
                            },
                        } as unknown as VerificationDetails,
                    }}
                    provider="postmark"
                />
            )
        )

        expect(screen.queryByText('DNS records to publish')).toBeNull()
    })
})
