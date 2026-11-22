import { expect, type Locator, type Page, test } from '@playwright/test'
import { login, navigateToPackage } from '@tinycld/core/e2e-helpers'

// The compose toolbar's Link dialog at a phone viewport. Below the mobile
// breakpoint Link folds into the toolbar's More actions menu and the dialog
// opens as a Sheet, which is different code from the desktop run in
// mail-buttons.spec.ts. The helpers below are copied from that spec, which
// keeps its own private.

const PHONE = { width: 390, height: 844 }

function composeWindow(page: Page): Locator {
    return page
        .locator('div')
        .filter({ has: page.getByRole('button', { name: 'Close compose', exact: true }) })
        .filter({ has: page.getByTestId('mail-compose-to') })
        .last()
}

function editor(page: Page): Locator {
    return page.locator('.tinycld-mail-editor .ProseMirror').filter({ visible: true }).first()
}

async function fittedToolbar(scope: Locator): Promise<Locator> {
    const toolbar = scope.getByTestId('toolbar').filter({ visible: true }).first()
    const row = toolbar.getByTestId('toolbar-row')
    await expect(row).toHaveCSS('opacity', '1')
    // After a viewport shrinks, the row shows the fit for the old width for a
    // frame or two; the fit for the current width is in place when every slot
    // ends inside the row (1px slack = fitToolbarItems' FIT_TOLERANCE).
    await expect
        .poll(() =>
            row.evaluate(el => {
                const right = el.getBoundingClientRect().right + 1
                return Array.from(el.children).every(c => c.getBoundingClientRect().right <= right)
            })
        )
        .toBe(true)
    return toolbar
}

async function expectOnScreen(locator: Locator) {
    await expect(locator).toBeVisible()
    await expect
        .poll(() =>
            locator.evaluate(el => {
                let opacity = 1
                for (let node: Element | null = el; node; node = node.parentElement) {
                    opacity *= Number(getComputedStyle(node).opacity)
                }
                return opacity
            })
        )
        .toBeGreaterThan(0.99)
    await expect(locator).toBeInViewport()
}

async function chooseMenuItem(page: Page, name: string) {
    const item = page.getByRole('menuitem', { name, exact: true }).filter({ visible: true }).last()
    await expectOnScreen(item)
    await item.click()
}

async function openLinkFromMore(page: Page, toolbar: Locator) {
    await toolbar.getByRole('button', { name: 'More actions', exact: true }).click()
    await chooseMenuItem(page, 'Link')
}

test.describe('email-share: compose link dialog', () => {
    test('at phone width Link in More sets a link and Cancel keeps it', async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
        await page.setViewportSize(PHONE)
        const fab = page.getByLabel('Compose email', { exact: true }).filter({ visible: true })
        await expect(fab).toBeVisible()

        await fab.click()
        await expect(page.getByTestId('mail-compose-to')).toBeVisible()
        await editor(page).click()
        await page.keyboard.type('linked')
        // Select All, not Shift+Home: see the Link test in mail-buttons.spec.ts.
        await page.keyboard.press('ControlOrMeta+a')

        const toolbar = await fittedToolbar(composeWindow(page))
        await expect(toolbar.getByRole('button', { name: 'Link', exact: true })).toBeHidden()

        const input = page.getByRole('textbox', { name: 'Link', exact: true })
        await openLinkFromMore(page, toolbar)
        await expect(input).toBeFocused()
        await input.fill('https://example.com/')
        await page
            .getByRole('button', { name: /^Apply/ })
            .filter({ visible: true })
            .click()
        await expect(input).toBeHidden()
        const link = editor(page).locator('a[href]')
        await expect(link).toHaveCount(1)
        await expect(link).toHaveAttribute('href', 'https://example.com/')
        await expect(link).toHaveText('linked')

        await openLinkFromMore(page, toolbar)
        await expect(input).toBeFocused()
        await input.fill('https://changed.example/')
        await page
            .getByRole('button', { name: /^Cancel/ })
            .filter({ visible: true })
            .click()
        await expect(input).toBeHidden()
        await expect(link).toHaveCount(1)
        await expect(link).toHaveAttribute('href', 'https://example.com/')
        await expect(link).toHaveText('linked')
    })
})
