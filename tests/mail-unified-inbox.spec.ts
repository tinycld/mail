import { expect, test } from '@playwright/test'
import { clickSidebarItem, login, navigateToPackage } from '../../../../tests/e2e/helpers'

test.describe('Mail — Unified inbox', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail')
    })

    test('sidebar shows All Inboxes entry when user has 2+ mailboxes', async ({ page }) => {
        // Seed creates a personal mailbox + shared "Support" mailbox for the
        // test user, so the unified entry should be visible.
        await expect(page.getByText('All Inboxes', { exact: true })).toBeVisible()
    })

    test('All Inboxes lists threads from both personal and shared mailboxes', async ({ page }) => {
        await clickSidebarItem(page, 'All Inboxes')
        await expect(page).toHaveURL(/folder=all-inboxes/)

        // Thread that lives in the personal mailbox (from THREADS seed)
        await expect(page.getByText('Q2 Product Roadmap Review').first()).toBeVisible()

        // Threads that live in the shared "Support" mailbox (from SHARED_THREADS seed)
        await expect(page.getByText('Refund request for order #84210').first()).toBeVisible()
        await expect(page.getByText('API rate limit question').first()).toBeVisible()
    })

    test('rows in All Inboxes show the originating mailbox label', async ({ page }) => {
        await clickSidebarItem(page, 'All Inboxes')
        await expect(page).toHaveURL(/folder=all-inboxes/)

        // The unified view tags each row with the source mailbox name. The
        // shared mailbox label comes from its display_name ("Support").
        // Scope to first matching row — even within email-row, recent
        // changes added inline action buttons that re-expose the same
        // strings, so a bare filter() can match more than one descendant.
        const supportRow = page
            .getByTestId('email-row')
            .filter({ hasText: 'Refund request for order #84210' })
            .first()
        await expect(supportRow.getByText('Support', { exact: true }).first()).toBeVisible()

        const personalRow = page
            .getByTestId('email-row')
            .filter({ hasText: 'Q2 Product Roadmap Review' })
            .first()
        await expect(personalRow.getByText('Personal', { exact: true }).first()).toBeVisible()
    })

    test('opening a thread from All Inboxes preserves the unified context on back', async ({
        page,
    }) => {
        await clickSidebarItem(page, 'All Inboxes')
        await expect(page).toHaveURL(/folder=all-inboxes/)

        await page.getByText('Refund request for order #84210').first().click()
        await expect(page).toHaveURL(/\/mail\//)

        await page.goBack()
        await expect(page).toHaveURL(/folder=all-inboxes/)
        await expect(page.getByText('Refund request for order #84210').first()).toBeVisible()
    })
})
