import { expect, test } from '@playwright/test'
import { login } from '@tinycld/core/e2e-helpers'
import { navigateToSharedInboxSettings } from './helpers'

test.describe('Mail — Shared mailbox creation', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToSharedInboxSettings(page)
    })

    test('creating a shared mailbox adds it with the creator as owner', async ({ page }) => {
        const address = `shared-${Date.now().toString(36)}`

        await page.getByText('New shared inbox', { exact: true }).click()
        await expect(page.getByText('Set up a shared inbox')).toBeVisible()

        await page.getByTestId('address').fill(address)
        await page.getByTestId('display_name').fill('Shared Test')

        await page.getByText('Create inbox', { exact: true }).click()

        // Drawer closes only when both inserts (mailbox + first owner member) succeed.
        await expect(page.getByText('Set up a shared inbox')).toBeHidden()
        await expect(page.getByText(address).first()).toBeVisible()
        await expect(page.getByText('Shared Test · 1 member')).toBeVisible()
    })
})
