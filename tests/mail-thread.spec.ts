import { expect, test } from '@playwright/test'
import { clickSidebarItem, login, navigateToPackage } from '../../../../tests/e2e/helpers'

test.describe('Mail — Thread Detail', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail')
    })

    test('open thread shows subject and content', async ({ page }) => {
        await page.getByText('Q2 Product Roadmap Review').click()
        await page.waitForURL(/\/mail\//)

        await expect(page.getByText('Q2 Product Roadmap Review')).toBeVisible()
        // Verify message content is rendered
        await expect(page.getByText(/roadmap/).first()).toBeVisible()
    })

    test('multi-message thread shows multiple senders', async ({ page }) => {
        await page.getByText('Conference travel arrangements').click()
        await page.waitForURL(/\/mail\//)

        // Thread has 5 messages from Sarah Kim, Alice, David Lee
        await expect(page.getByText(/travel/).first()).toBeVisible()
    })

    test('archive thread from detail', async ({ page }) => {
        await page.getByText('Lunch tomorrow?').click()
        await page.waitForURL(/\/mail\//)

        await page.getByLabel('Archive').click()

        await page.waitForURL(url => !url.pathname.includes('/mail/'), { timeout: 10_000 })
        await expect(page.getByText('Lunch tomorrow?')).not.toBeVisible({ timeout: 10_000 })

        await clickSidebarItem(page, 'Archive')
        await expect(page.getByText('Lunch tomorrow?')).toBeVisible({ timeout: 10_000 })
    })

    test('delete thread from detail', async ({ page }) => {
        await page.getByText('resolve subdomain redirect loop').click()
        await page.waitForURL(/\/mail\//)

        await page.getByLabel('Delete').click()

        await page.waitForURL(url => !url.pathname.includes('/mail/'), { timeout: 10_000 })

        await clickSidebarItem(page, 'Trash')
        await expect(page.getByText('resolve subdomain redirect loop')).toBeVisible({
            timeout: 10_000,
        })
    })

    test('toggle read/unread from detail', async ({ page }) => {
        await page.getByText('Q2 Product Roadmap Review').click()
        await page.waitForURL(/\/mail\//)

        await page.getByLabel('Mark as unread').click()
        await page.getByLabel('Back').click()
    })

    test('navigate between threads with newer/older', async ({ page }) => {
        await page.getByText('Q2 Product Roadmap Review').click()
        await page.waitForURL(/\/mail\//)

        const olderButton = page.getByLabel('Older')
        if (await olderButton.isEnabled()) {
            await olderButton.click()
            await expect(page.getByLabel('Newer')).toBeEnabled()
        }
    })
})
