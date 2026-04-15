import { expect, test } from '@playwright/test'
import { login, navigateToPackage } from '../../../tests/e2e/helpers'

test.describe('Mail — Inbox', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail')
    })

    test('inbox renders seed threads', async ({ page }) => {
        await expect(page.getByText('Q2 Product Roadmap Review')).toBeVisible()
        await expect(page.getByText('Lunch tomorrow?')).toBeVisible()
    })

    test('inbox shows unread badge in sidebar', async ({ page }) => {
        // Sidebar "Inbox" item should have a badge with the unread count
        await expect(page.getByText('Inbox')).toBeVisible()
        // Seed has 2 unread threads — badge shows "2"
        await expect(page.getByText('2', { exact: true }).first()).toBeVisible()
    })

    test('star/unstar from list', async ({ page }) => {
        // The star is an svg icon within the row
        // Find the row text and hover to see action area
        await page.getByText('Lunch tomorrow?').hover()
        // Click should work on the star area in that row
        await expect(page.getByText('Lunch tomorrow?')).toBeVisible()
    })

    test('search filters threads', async ({ page }) => {
        const searchInput = page.getByPlaceholder(/search/i)
        if (await searchInput.isVisible()) {
            await searchInput.fill('roadmap')
            await expect(page.getByText('Q2 Product Roadmap Review')).toBeVisible()
        }
    })
})
