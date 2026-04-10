import { expect, test } from '@playwright/test'
import { login, navigateToAddon } from '../../../tests/e2e/helpers'

test.describe('Mail — Compose', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToAddon(page, 'mail')
    })

    test('open compose from sidebar', async ({ page }) => {
        await page.getByText('Compose', { exact: true }).click()

        // Compose window should appear with To and Subject labels
        await expect(page.getByText('To', { exact: true })).toBeVisible()
        await expect(page.getByText('Subject', { exact: true })).toBeVisible()
    })

    test('compose shows send button', async ({ page }) => {
        await page.getByText('Compose', { exact: true }).click()
        await expect(page.getByText('To', { exact: true })).toBeVisible()
        // The Send button is a pill-shaped accent button
        await expect(page.getByText('Send', { exact: true })).toBeVisible()
    })

    test('close compose without sending', async ({ page }) => {
        await page.getByText('Compose', { exact: true }).click()
        await expect(page.getByText('To', { exact: true })).toBeVisible()

        // Close via Escape
        await page.keyboard.press('Escape')
    })
})
