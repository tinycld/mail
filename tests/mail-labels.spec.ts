import { expect, test } from '@playwright/test'
import { login, navigateToPackage } from '@tinycld/core/e2e-helpers'
import { deliverInbound, openThread, uniqueSubject } from './helpers'

test.describe('Mail — Labels', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
    })

    test('filter by label in sidebar navigates to label-scoped URL', async ({ page }) => {
        // Each label row carries a stable testID — no DOM-structure walking
        // (the old ancestor::*[5] xpath broke on any layout change and could
        // match unrelated text).
        await page
            .getByTestId(/^mail-sidebar-label-/)
            .filter({ hasText: 'Work' })
            .first()
            .click()
        await expect(page).toHaveURL(/label=/)
    })

    test('thread detail shows labels toolbar', async ({ page, request }) => {
        const subject = uniqueSubject('LabelsToolbar')
        await deliverInbound(request, { subject })
        await page.reload()

        await openThread(page, subject)
        await expect(page.getByLabel('Labels').first()).toBeVisible()
    })
})
