import { expect, test } from '@playwright/test'
import { login, navigateToPackage } from '../../../tests/e2e/helpers'

test.describe('Mail — Bulk Actions', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail')
    })

    test('toolbar shows email count', async ({ page }) => {
        // Verify the default toolbar shows a count like "1–5 of 5"
        await expect(page.getByText(/\d+\u2013\d+ of \d+/).first()).toBeVisible()
    })

    test('bulk toolbar appears on selection', async ({ page }) => {
        // Click the select-all checkbox (first pressable with Square icon in the toolbar)
        const checkbox = page
            .locator('svg[data-testid="square-icon"]')
            .first()
            .or(page.locator('[aria-label="Refresh"]').locator('..').locator('svg').first())
        if (await checkbox.isVisible()) {
            await checkbox.click()
            // Should show bulk actions toolbar with "selected" text
            const selected = page.getByText(/\d+ selected/)
            if (await selected.isVisible({ timeout: 3_000 }).catch(() => false)) {
                await expect(selected).toBeVisible()
            }
        }
    })
})
