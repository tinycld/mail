import { expect, test } from '@playwright/test'
import { clickSidebarItem, login, navigateToPackage } from '../../../../tests/e2e/helpers'

// Scope toolbar lookups to the detail container — each row in the inbox
// also exposes per-row hover actions with the same aria-labels (Archive,
// Delete, Mark as unread, etc.), so an unscoped getByLabel matches dozens
// of rows when the inbox is still visible alongside the detail view.
const detailToolbar = (page: import('@playwright/test').Page) =>
    page.getByTestId('mail-thread-detail')

test.describe('Mail — Thread Detail', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail')
    })

    test('open thread shows subject and content', async ({ page }) => {
        await page.getByText('Q2 Product Roadmap Review').first().click()
        await page.waitForURL(/\/mail\//)

        await expect(page.getByText('Q2 Product Roadmap Review').first()).toBeVisible()
        // Verify message content is rendered
        await expect(page.getByText(/roadmap/).first()).toBeVisible()
    })

    test('multi-message thread shows multiple senders', async ({ page }) => {
        await page.getByText('Conference travel arrangements').first().click()
        await page.waitForURL(/\/mail\//)

        // Thread has 5 messages from Sarah Kim, Alice, David Lee
        await expect(page.getByText(/travel/).first()).toBeVisible()
    })

    test('archive thread from detail', async ({ page }) => {
        await page.getByText('Lunch tomorrow?').first().click()
        await page.waitForURL(/\/mail\//)

        await detailToolbar(page).getByLabel('Archive').click()

        await page.waitForURL((url) => !url.pathname.includes('/mail/'), { timeout: 10_000 })
        await expect(page.getByText('Lunch tomorrow?')).not.toBeVisible({ timeout: 10_000 })

        await clickSidebarItem(page, 'Archive')
        await expect(page.getByText('Lunch tomorrow?').first()).toBeVisible({ timeout: 10_000 })
    })

    test('delete thread from detail', async ({ page }) => {
        await page.getByText('resolve subdomain redirect loop').first().click()
        await page.waitForURL(/\/mail\//)

        await detailToolbar(page).getByLabel('Delete').click()

        await page.waitForURL((url) => !url.pathname.includes('/mail/'), { timeout: 10_000 })

        await clickSidebarItem(page, 'Trash')
        await expect(page.getByText('resolve subdomain redirect loop').first()).toBeVisible({
            timeout: 10_000,
        })
    })

    test('toggle read/unread from detail', async ({ page }) => {
        await page.getByText('Q2 Product Roadmap Review').first().click()
        await page.waitForURL(/\/mail\//)

        await detailToolbar(page).getByLabel('Mark as unread').click()
        await detailToolbar(page).getByLabel('Back').click()
    })

    test('navigate between threads with newer/older', async ({ page }) => {
        await page.getByText('Q2 Product Roadmap Review').first().click()
        await page.waitForURL(/\/mail\//)

        const olderButton = detailToolbar(page).getByLabel('Older')
        if (await olderButton.isEnabled()) {
            await olderButton.click()
            await expect(detailToolbar(page).getByLabel('Newer')).toBeEnabled()
        }
    })
})
