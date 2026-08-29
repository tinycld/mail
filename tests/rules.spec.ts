import { expect, type Page, test } from '@playwright/test'
import { clickSidebarItem, login, navigateToPackage } from '@tinycld/core/e2e-helpers'
import { deliverInbound, emailRow, navigateToPersonalInbox } from './helpers'

// Proves mail's rule loop end to end: a rule built in the real builder, a
// message delivered through mail's real inbound webhook, and the visible
// effect in the UI — the thread moving out of Inbox and into Archive — plus
// the rule_runs entry behind it.
//
// The visible effect is the point. An action that writes rows no view reads
// is worse than no action, so asserting the folder change (not just a
// database row) is what makes this test worth having.

// Element-gated settings navigation, mirroring navigateToMailboxSettings in
// ./helpers — never page.goto(), which tears down the SPA mid-navigation.
async function navigateToRulesSettings(page: Page) {
    await page.getByTestId('nav-settings').click()
    await page.getByText('Rules', { exact: true }).first().click()
    await expect(page.getByText('My rules', { exact: true })).toBeVisible()
}

// The builder's pickers are the house Menu component (a Pressable trigger
// plus text-labeled items, not a native <select>), so drive them by label.
async function selectFromMenu(
    page: Page,
    trigger: import('@playwright/test').Locator,
    optionLabel: string
) {
    await trigger.click()
    await page.getByText(optionLabel, { exact: true }).click()
}

// Find a rule's row by the nearest ancestor that also holds its overflow
// trigger — resilient to how deeply the name Text is nested.
function ruleRow(page: Page, ruleName: string) {
    return page
        .locator('div')
        .filter({ has: page.getByText(ruleName, { exact: true }) })
        .filter({ has: page.getByLabel('More actions') })
        .last()
}

test.describe('Mail — Rules', () => {
    test('an arriving message is filed by a rule, and the run is logged', async ({
        page,
        request,
    }) => {
        await login(page)

        const stamp = Date.now()
        const ruleName = `E2E mail filter ${stamp}`
        // The condition keys on this marker so the rule only touches the
        // message this test delivers — parallel specs share the mailbox.
        const marker = `rulesubj${stamp}`

        await navigateToRulesSettings(page)

        await page.getByText('New rule', { exact: true }).first().click()
        await expect(page.getByText('New rule', { exact: true }).last()).toBeVisible()
        await page.getByPlaceholder('Rule name').fill(ruleName)

        await selectFromMenu(
            page,
            page.getByText('Select a trigger…', { exact: true }),
            'A message arrives'
        )

        // Narrow to this test's message: subject contains the marker. Choosing a
        // record trigger offers a ready condition row, so the field picker is
        // there immediately — no "add OR group" then "add condition" first.
        await selectFromMenu(page, page.getByText('Field…', { exact: true }), 'Subject')
        // "contains" is the default op for a text field, so only the value is
        // left to fill. The row's textbox is the sole input in the IF card.
        await page.getByRole('textbox').last().fill(marker)

        await page.getByText('add action', { exact: true }).click()
        await page.getByText('Move to folder', { exact: true }).click()
        await selectFromMenu(page, page.getByText('Select…', { exact: true }).last(), 'archive')

        await page.getByText('Save', { exact: true }).click()
        await expect(page.getByText(ruleName, { exact: true })).toBeVisible()

        // Real ingress: mail's Postmark inbound webhook, never a raw PB write.
        const { subject } = await deliverInbound(request, {
            subject: `Invoice ${marker} attached`,
        })

        // The visible effect: the thread is filed under Archive...
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
        await navigateToPersonalInbox(page)
        await clickSidebarItem(page, 'Archive')
        await expect(emailRow(page, subject)).toBeVisible({ timeout: 20_000 })

        // ...and gone from the Inbox it would otherwise have landed in.
        await clickSidebarItem(page, 'Inbox')
        await expect(emailRow(page, subject)).toBeHidden()

        // And the run is recorded, so the user can see why it moved.
        await navigateToRulesSettings(page)
        await ruleRow(page, ruleName).getByLabel('More actions').click()
        await page.getByText('Run history', { exact: true }).click()
        await expect(page.getByText('Matched', { exact: true }).first()).toBeVisible({
            timeout: 15_000,
        })
    })

    test('the mail rules help topic is searchable and renders', async ({ page }) => {
        await login(page)

        await page.getByTestId('nav-help').click()
        await expect(page).toHaveURL(/\/help$/)

        await page.getByPlaceholder('Search help topics').fill('mail rules')
        await page.getByText('Mail rules', { exact: true }).click()

        await expect(page).toHaveURL(/\/help\/mail\/rules$/)
        await expect(page.getByText('When a message arrives', { exact: true })).toBeVisible()
    })
})
