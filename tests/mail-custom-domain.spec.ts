import { expect, type Page, test } from '@playwright/test'
import { login, navigateToPackage } from '@tinycld/core/e2e-helpers'
import PocketBase from 'pocketbase'

// A real Postmark account is not available in CI, and POST /api/mail/domains
// enrolls with the mail provider before it creates the row — see
// mail/server/endpoints_add_domain.go. Without provider credentials that
// call always fails (ErrNotConfigured), so this spec stubs the endpoint at
// the network layer rather than driving a request that cannot succeed here.
//
// The settings screen never reads the mutation's response body — the DNS
// records panel is fed entirely by the live-synced mail_domains row (see
// DomainsSection / DomainVerificationPanel in
// tinycld/mail/settings/provider.tsx). So the stub does what the real
// handler does: it creates the mail_domains row (as the PocketBase
// superuser, mirroring the seeding pattern in mail-shared-mailbox-admin.spec.ts
// and helpers.ts's deliverInbound) with the same verification_details shape
// the real endpoint populates, then resolves the intercepted request with a
// matching AddDomainResponse. The UI is still driven normally: the spec
// fills in the domain and clicks Add exactly as a user would, and the panel
// that appears is the real component rendering real (locally-created) data,
// not an assertion against the fixture's JSON.
//
// Because it intercepts POST /api/mail/domains at the network layer, this
// spec does NOT exercise the real handleAddDomain server path (provider
// enrollment, ErrNotConfigured / ErrDomainAlreadyEnrolled handling, the
// actual DNS record values Postmark returns). Do not read this spec as
// end-to-end coverage of domain enrollment — it only covers the client-side
// settings UI rendering a mail_domains row.

// PB sits behind the dev.ts proxy on the test Expo port. /api/* routes
// through to PB transparently — see scripts/dev.ts::isPbPath.
const PB_URL = 'http://127.0.0.1:7200'
const SUPERUSER_EMAIL = process.env.POCKETBASE_EMAIL || 'admin@tinycld.org'
const SUPERUSER_PASSWORD = process.env.POCKETBASE_PASSWORD || 'AdminPass1234!'

interface StubbedOutbound {
    dkim_host: string
    dkim_text_value: string
    return_path_domain: string
    return_path_cname_value: string
}

// Mirrors what handleAddDomain (mail/server/endpoints_add_domain.go) writes
// on a successful enrollment: the outbound DNS records are populated
// immediately, MX/provider checks are left false (they haven't run yet).
function buildVerificationDetails(outbound: StubbedOutbound) {
    return {
        mx: { ok: false, expected: '' },
        provider: { ok: false },
        outbound: {
            spf: false,
            dkim: false,
            return_path: false,
            enrolled: 'yes',
            ...outbound,
        },
        provider_configured: true,
        provider_name: 'postmark',
    }
}

// Creates the mail_domains row a successful enrollment would have created,
// using the same superuser-authenticated PocketBase client pattern as
// deliverInbound and seedUser elsewhere in this test suite.
async function seedEnrolledDomain(domain: string, outbound: StubbedOutbound) {
    const pb = new PocketBase(PB_URL)
    await pb.collection('_superusers').authWithPassword(SUPERUSER_EMAIL, SUPERUSER_PASSWORD)
    const record = await pb.collection('mail_domains').create({
        domain,
        verified: false,
        mx_verified: false,
        inbound_domain_verified: false,
        spf_verified: false,
        dkim_verified: false,
        return_path_verified: false,
        verification_details: buildVerificationDetails(outbound),
    })
    return record.id as string
}

// Intercepts the add-domain request, seeds the matching row, and resolves
// with the AddDomainResponse shape the real handler returns — so the client
// sees the same success contract it would from a real enrollment.
async function stubAddDomain(page: Page, domain: string, outbound: StubbedOutbound) {
    await page.route('**/api/mail/domains', async route => {
        if (route.request().method() !== 'POST') {
            await route.continue()
            return
        }
        const id = await seedEnrolledDomain(domain, outbound)
        await route.fulfill({
            status: 200,
            contentType: 'application/json',
            body: JSON.stringify({
                id,
                domain,
                records: {
                    enrolled: 'yes',
                    spf: false,
                    dkim: false,
                    return_path: false,
                    ...outbound,
                },
            }),
        })
    })
}

async function navigateToDomainSettings(page: Page) {
    await navigateToPackage(page, 'mail', { waitFor: page.getByTestId('package-sidebar-mounted') })
    await page.getByTestId('nav-settings').click()
    await page.getByText('Domains', { exact: true }).first().click()
    await expect(page.getByText('Mail Domains', { exact: true })).toBeVisible()
}

test.describe('Mail — Custom domain DNS records', () => {
    test('adding a domain shows the DNS records to publish, with a copyable value', async ({
        page,
    }) => {
        const stamp = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
        const domain = `custom-domain-${stamp}.example.com`
        const outbound: StubbedOutbound = {
            dkim_host: `pm._domainkey.${domain}`,
            dkim_text_value: 'k=rsa;p=stub-dkim-public-key-value',
            return_path_domain: `pm-bounces.${domain}`,
            return_path_cname_value: 'pm.mtasv.net',
        }

        await stubAddDomain(page, domain, outbound)

        await login(page)
        await navigateToDomainSettings(page)

        await page.getByTestId('domain').fill(domain)
        await page.getByText('Add', { exact: true }).click()

        // The DomainRowItem card for the domain we just added — the nearest
        // ancestor that carries both the domain name and the DKIM label
        // that panel renders, so assertions below can't match a different
        // domain's card (mirrors ruleRow in mail-rules-screen.spec.ts).
        const row = page
            .locator('div')
            .filter({ has: page.getByText(domain, { exact: true }) })
            .filter({ has: page.getByText('DKIM (TXT)', { exact: true }) })
            .last()
        await expect(row.getByText('DNS records to publish', { exact: true })).toBeVisible()

        // The DKIM record's host and value appear once each within this
        // domain's card — no further scoping needed.
        await expect(row.getByText(outbound.dkim_host, { exact: true })).toBeVisible()
        await expect(row.getByText(outbound.dkim_text_value, { exact: true })).toBeVisible()

        // The Copy control is what makes this a copyable value, not just
        // text on the page — headless CI has no clipboard permission
        // granted, so assert the control is present and clickable rather
        // than asserting the OS clipboard actually received anything.
        const copyButton = row.getByLabel('Copy DKIM value')
        await expect(copyButton).toBeVisible()
        await copyButton.click()
    })
})
