import { expect, test } from '@playwright/test'
import { login, navigateToPackage } from '@tinycld/core/e2e-helpers'

test.describe('Mail — Compose', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
    })

    test('open compose from sidebar', async ({ page }) => {
        await page.getByText('Compose', { exact: true }).click()

        // The compose drawer mounts as a lazy chunk; first paint can
        // exceed the default 5s on CI under load. Once the first field
        // ("To") is up the rest are in the same render frame.
        await expect(page.getByText('To', { exact: true })).toBeVisible()
        await expect(page.getByText('Subject', { exact: true })).toBeVisible()
        await expect(page.getByText('Send', { exact: true })).toBeVisible()
    })

    test('close compose without sending', async ({ page }) => {
        await page.getByText('Compose', { exact: true }).click()
        await expect(page.getByText('To', { exact: true })).toBeVisible()

        await page.keyboard.press('Escape')
    })

    // The body editor was rendering as a narrow, centered column (~680px) instead
    // of filling the compose width (mail #2). Assert the editable spans most of
    // its container so it can't regress to a centered sliver.
    test('body editor fills the compose width', async ({ page }) => {
        await page.setViewportSize({ width: 1400, height: 900 })
        await page.getByText('Compose', { exact: true }).click()
        await expect(page.getByText('To', { exact: true })).toBeVisible()

        const wrapper = page.locator('.tinycld-mail-editor')
        const editable = page.locator('.tinycld-mail-editor .ProseMirror')
        await expect(editable).toBeVisible()

        const wrapperBox = await wrapper.boundingBox()
        const editableBox = await editable.boundingBox()
        if (!wrapperBox || !editableBox) throw new Error('editor not laid out')

        // Full-width: the editable should span essentially the whole wrapper, not
        // a centered column. Allow a small margin for padding/borders.
        expect(editableBox.width).toBeGreaterThan(wrapperBox.width - 8)
    })
})
