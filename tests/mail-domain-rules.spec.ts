// mail-domain-rules.spec.ts — the domain rule gives every person an address.
//
// The second domain is seeded VERIFIED by a superuser write. Verification
// needs a live DNS MX lookup and provider enrollment, which CI does not have,
// so the add-and-verify path cannot run here (mail-custom-domain.spec.ts
// stubs the add endpoint for the same reason).
// This spec therefore does NOT cover adding or verifying a domain.
// Everything after the seed — changing the rule, the preview, the new
// address in the mailbox and in compose — is driven through the UI.
import { expect, test } from '@playwright/test'
import { login, navigateToPackage } from '@tinycld/core/e2e-helpers'
import { navigateToMailboxSettings, seedVerifiedDomain, TEST_USER_MAILBOX } from './helpers'

test.describe('Mail — Domain rule', () => {
    test('setting a domain to Everyone gives the test user an address on it', async ({ page }) => {
        const stamp = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
        const domain = `rules-${stamp}.example.com`
        const localPart = TEST_USER_MAILBOX.split('@')[0]
        const address = `${localPart}@${domain}`

        await seedVerifiedDomain(domain)

        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
        await page.getByTestId('nav-settings').click()
        await page.getByText('Domains', { exact: true }).first().click()
        await expect(page.getByText('Mail Domains', { exact: true })).toBeVisible()

        // The nearest ancestor holding this domain's name and its rule control,
        // so a click cannot land in another domain's card.
        const card = page
            .locator('div')
            .filter({ has: page.getByText(domain, { exact: true }) })
            .filter({ has: page.getByText('Who gets an address here?', { exact: true }) })
            .last()
        await expect(card.getByText('Only people I add', { exact: true })).toBeVisible()

        await card.getByText('Everyone', { exact: true }).click()
        await expect(card.getByText(new RegExp(`^Adds .*@${domain}`))).toBeVisible()
        await card.getByText('Apply', { exact: true }).click()
        await expect(card.getByText('Apply', { exact: true })).toBeHidden()

        await navigateToMailboxSettings(page)
        await page.getByTestId(`mailbox-row-${TEST_USER_MAILBOX}`).click()
        const drawer = page.getByRole('dialog')
        await expect(drawer.getByText('Addresses', { exact: true })).toBeVisible()
        await expect(drawer.getByText(address, { exact: true })).toBeVisible()
        await expect(drawer.getByText('From the domain rule', { exact: true })).toBeVisible()
        // The drawer is modal: its backdrop covers the app rail until it closes.
        await drawer.getByRole('button', { name: 'Close' }).click()
        await expect(drawer).toBeHidden()

        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
        await page.getByText('Compose', { exact: true }).click()
        await expect(page.getByText('To', { exact: true })).toBeVisible()
        await page.getByText('Change', { exact: true }).click()
        await expect(page.getByText(address, { exact: true })).toBeVisible()
    })
})
