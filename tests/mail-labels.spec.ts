import { expect, test } from '@playwright/test'
import { login, navigateToPackage } from '../../tinycld/tests/e2e/helpers'
import { deliverInbound, openThread, uniqueSubject } from './helpers'

test.describe('Mail — Labels', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
    })

    test('filter by label in sidebar navigates to label-scoped URL', async ({ page }) => {
        const labelsSection = page.getByText('Labels').locator('xpath=ancestor::*[5]')
        await labelsSection.getByText('Work', { exact: true }).click()
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
