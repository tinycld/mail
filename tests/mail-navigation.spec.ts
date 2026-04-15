import { expect, test } from '@playwright/test'
import { clickSidebarItem, login, navigateToPackage } from '../../../tests/e2e/helpers'

test.describe('Mail — Navigation', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail')
    })

    test('navigate to Sent', async ({ page }) => {
        await clickSidebarItem(page, 'Sent')
        await expect(page).toHaveURL(/folder=sent/)
        await expect(page.getByText('Team standup notes').first()).toBeVisible()
    })

    test('navigate to Drafts', async ({ page }) => {
        await clickSidebarItem(page, 'Drafts')
        await expect(page).toHaveURL(/folder=drafts/)
        await expect(page.getByText('Draft: Monthly newsletter')).toBeVisible()
    })

    test('navigate to Starred', async ({ page }) => {
        await clickSidebarItem(page, 'Starred')
        await expect(page).toHaveURL(/folder=starred/)
        // Seed has 2 starred threads
        await expect(page.getByText('Q2 Product Roadmap Review')).toBeVisible()
        await expect(page.getByText('Conference travel arrangements')).toBeVisible()
    })

    test('navigate to Trash shows trash items', async ({ page }) => {
        await clickSidebarItem(page, 'Trash')
        await expect(page).toHaveURL(/folder=trash/)
        await expect(page.getByText('Old project files').first()).toBeVisible()
    })

    test('navigate to Spam shows spam items', async ({ page }) => {
        await clickSidebarItem(page, 'Spam')
        await expect(page).toHaveURL(/folder=spam/)
        await expect(page.getByText('won a free cruise').first()).toBeVisible()
    })

    test('navigate to All Mail', async ({ page }) => {
        await clickSidebarItem(page, 'All Mail')
        await expect(page).toHaveURL(/folder=all/)
        // All Mail should show threads from multiple folders
        await expect(page.getByText('Q2 Product Roadmap Review')).toBeVisible()
    })
})
