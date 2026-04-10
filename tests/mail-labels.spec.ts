import { expect, test } from '@playwright/test'
import { login, navigateToAddon } from '../../../tests/e2e/helpers'

test.describe('Mail — Labels', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToAddon(page, 'mail')
    })

    test('sidebar shows labels section', async ({ page }) => {
        await expect(page.getByText('Labels')).toBeVisible()
        await expect(page.getByText('Work')).toBeVisible()
        await expect(page.getByText('Personal').first()).toBeVisible()
    })

    test('filter by label in sidebar', async ({ page }) => {
        await page.getByText('Work').click()
        await expect(page).toHaveURL(/label=/)

        await expect(page.getByText('Q2 Product Roadmap Review')).toBeVisible()
    })

    test('thread detail shows labels toolbar', async ({ page }) => {
        await page.getByText('Lunch tomorrow?').click()
        await page.waitForURL(/\/mail\//)

        // Labels toolbar button should be visible
        await expect(page.getByLabel('Labels').first()).toBeVisible()
    })
})
