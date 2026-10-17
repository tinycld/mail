// mail-client-setup.spec.ts — Settings → Account → Mail apps.
//
// Read-only: the test user opens the page and sees the settings of their
// Default From address. The page changes nothing, so no cleanup is needed.
// The profile and discovery answers are covered by the Go tests
// (server/client_profile_test.go, server/client_discovery_test.go).
import { expect, test } from '@playwright/test'
import { login } from '@tinycld/core/e2e-helpers'
import { TEST_USER_MAILBOX } from './helpers'

test('the Mail apps page shows the settings of my address', async ({ page }) => {
    await login(page)
    await page.getByTestId('nav-settings').click()
    // Scoped to the Account group, as in mail-address-views.spec.ts.
    const accountGroup = page.getByText('Account', { exact: true }).locator('..')
    await accountGroup.getByText('Mail apps', { exact: true }).click()

    await expect(page.getByTestId('mail-app-address')).toHaveText(TEST_USER_MAILBOX)
    await expect(page.getByTestId('mail-app-user-name')).toHaveText(TEST_USER_MAILBOX)
    await expect(page.getByTestId('mail-app-imap-host')).not.toBeEmpty()
    await expect(page.getByText('Port 993, SSL/TLS')).toBeVisible()
    await expect(page.getByText('Port 465, SSL/TLS')).toBeVisible()
    await expect(page.getByLabel('QR code that opens this page on your phone')).toBeVisible()
    await expect(page.getByTestId('mail-app-profile-button')).toBeEnabled()
})
