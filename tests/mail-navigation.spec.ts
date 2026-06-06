import { expect, test } from '@playwright/test'
import {
    clickSidebarItem,
    login,
    navigateToPackage,
    ORG_SLUG,
} from '../../tinycld/tests/e2e/helpers'

// These tests assert that each folder route mounts successfully — i.e.
// clicking the sidebar entry navigates to the right URL. Asserting on
// specific seeded thread subjects is fragile because parallel specs
// archive/delete/trash threads and the folder contents drift.

test.describe('Mail — Navigation', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
    })

    test('j/k move focus and Enter opens the focused mail row', async ({ page }) => {
        // Mail-specific keyboard nav: j moves focus down the inbox row
        // list, k moves up, Enter opens the focused row. Lives here in
        // mail's repo (not app's) because it's exercising mail's own
        // inbox-row focus behavior; app-shell keyboard tests live in
        // app/tests/e2e/keyboard-shortcuts.spec.ts.
        await page.mouse.click(10, 10)
        // Inbox rows hydrate from a live query; on CI under load that
        // first paint can exceed 10s. FlashList renders some rows off-
        // screen — the `:visible` filter waits for one that actually
        // got positioned in the viewport.
        await page
            .locator('[data-testid="email-row"]:visible')
            .first()
            .waitFor({ state: 'visible', timeout: 30_000 })

        await page.keyboard.press('j')
        await page.keyboard.press('j')
        await page.keyboard.press('Enter')

        await page.waitForURL(new RegExp(`/a/${ORG_SLUG}/mail/[^/?]+`), { timeout: 5_000 })
    })

    test('navigate to Sent', async ({ page }) => {
        await clickSidebarItem(page, 'Sent')
        await expect(page).toHaveURL(/folder=sent/)
    })

    test('navigate to Drafts', async ({ page }) => {
        await clickSidebarItem(page, 'Drafts')
        await expect(page).toHaveURL(/folder=drafts/)
    })

    test('navigate to Starred', async ({ page }) => {
        await clickSidebarItem(page, 'Starred')
        await expect(page).toHaveURL(/folder=starred/)
    })

    test('navigate to Trash', async ({ page }) => {
        await clickSidebarItem(page, 'Trash')
        await expect(page).toHaveURL(/folder=trash/)
    })

    test('navigate to Spam', async ({ page }) => {
        await clickSidebarItem(page, 'Spam')
        await expect(page).toHaveURL(/folder=spam/)
    })
})
