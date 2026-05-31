import { expect, test } from '@playwright/test'
import { login, navigateToPackage } from '../../app/tests/e2e/helpers'

test.describe('Mail — Compose', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
    })

    test('open compose from sidebar', async ({ page }) => {
        await page.getByText('Compose', { exact: true }).click()

        // The compose drawer mounts as a lazy chunk; first paint can
        // exceed the default 5s on CI under load. Once the first field
        // ("To") is up the rest are in the same render frame.
        await expect(page.getByText('To', { exact: true })).toBeVisible({ timeout: 15_000 })
        await expect(page.getByText('Subject', { exact: true })).toBeVisible()
        await expect(page.getByText('Send', { exact: true })).toBeVisible()
    })

    test('close compose without sending', async ({ page }) => {
        await page.getByText('Compose', { exact: true }).click()
        await expect(page.getByText('To', { exact: true })).toBeVisible({ timeout: 15_000 })

        await page.keyboard.press('Escape')
    })
})
