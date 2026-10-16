// mail-address-views.spec.ts — a folder for each address in the sidebar.
//
// Everything runs on the main domain (tinycld.org) and is driven through the
// UI: an admin gives the test user a second address in Members, the user
// turns on "Show a folder for each address" in Account → Email, and the
// sidebar views list only the mail that used their address. Other specs share
// the test user, so an afterEach puts it back as it was (switch off, address
// removed), also after a failed step.
import { expect, type Page, test } from '@playwright/test'
import { login, navigateToPackage } from '@tinycld/core/e2e-helpers'
import {
    deliverInbound,
    emailRow,
    expectRowVisible,
    openPersonEmail,
    TEST_USER_MAILBOX,
    TEST_USER_USERNAME,
    uniqueSubject,
} from './helpers'

const MAIN_DOMAIN = 'tinycld.org'
const VIEWS_SWITCH = 'Show a folder for each address'

function addressView(page: Page, email: string) {
    return page.getByTestId(`address-view-${email}`)
}

async function openMail(page: Page) {
    await navigateToPackage(page, 'mail', {
        waitFor: page.getByTestId('package-sidebar-mounted'),
    })
}

// Settings → Account → Email. Scoped to the Account group: "Email" is also
// the name of a section in the Members drawer.
async function openAccountEmail(page: Page) {
    await page.getByTestId('nav-settings').click()
    const accountGroup = page.getByText('Account', { exact: true }).locator('..')
    await accountGroup.getByText('Email', { exact: true }).click()
}

async function setAddressViews(page: Page, on: boolean) {
    await openAccountEmail(page)
    const viewsSwitch = page.getByRole('switch', { name: VIEWS_SWITCH })
    await expect(viewsSwitch).toHaveAttribute('aria-checked', String(!on))
    await viewsSwitch.click()
    await expect(viewsSwitch).toHaveAttribute('aria-checked', String(on))
}

async function addAddress(page: Page, localPart: string) {
    const drawer = await openPersonEmail(page, TEST_USER_USERNAME)
    await drawer.getByText('Add address', { exact: true }).click()
    await drawer.getByTestId('local_part').fill(localPart)
    // Other specs add domains while this one runs, so pick the domain by name.
    await drawer
        .getByRole('radiogroup', { name: 'Domain' })
        .getByRole('radio', { name: MAIN_DOMAIN, exact: true })
        .click()
    await drawer.getByText('Add address', { exact: true }).click()
    await expect(drawer.getByTestId(`address-row-${localPart}@${MAIN_DOMAIN}`)).toBeVisible()
    // The drawer is modal: its backdrop covers the app rail until it closes.
    await drawer.getByRole('button', { name: 'Close' }).click()
    await expect(drawer).toBeHidden()
}

// The cleanup runs after a failed step too, so it reads the state first and
// changes only what the test left behind. Other specs share the test user.
async function restoreTestUser(page: Page, addedAddress: string) {
    const openDrawer = page.getByRole('dialog').first()
    if (await openDrawer.isVisible()) {
        await openDrawer.getByRole('button', { name: 'Close' }).click()
        await expect(openDrawer).toBeHidden()
    }

    await openAccountEmail(page)
    // The main address row proves the panel loaded, so a missing or off
    // switch below is an answer.
    await expect(page.getByText(TEST_USER_MAILBOX, { exact: true }).first()).toBeVisible()
    const viewsSwitch = page.getByRole('switch', { name: VIEWS_SWITCH })
    if (
        (await viewsSwitch.count()) > 0 &&
        (await viewsSwitch.getAttribute('aria-checked')) === 'true'
    ) {
        await viewsSwitch.click()
        await expect(viewsSwitch).toHaveAttribute('aria-checked', 'false')
    }

    const drawer = await openPersonEmail(page, TEST_USER_USERNAME)
    await expect(drawer.getByTestId(`address-row-${TEST_USER_MAILBOX}`)).toBeVisible()
    const row = drawer.getByTestId(`address-row-${addedAddress}`)
    if ((await row.count()) > 0) {
        await row.getByRole('button', { name: `Remove address ${addedAddress}` }).click()
        await expect(row).toBeHidden()
    }
    await drawer.getByRole('button', { name: 'Close' }).click()
    await expect(drawer).toBeHidden()
}

test.describe('Mail — A folder for each address', () => {
    const stamp = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
    const localPart = `v${stamp}`
    const secondAddress = `${localPart}@${MAIN_DOMAIN}`
    let isSignedIn = false

    test.afterEach(async ({ page }) => {
        if (isSignedIn) await restoreTestUser(page, secondAddress)
    })

    test('each address view lists only the mail sent to that address', async ({
        page,
        request,
    }) => {
        await login(page)
        isSignedIn = true
        await openMail(page)
        await addAddress(page, localPart)

        await setAddressViews(page, true)
        await openMail(page)
        await expect(addressView(page, TEST_USER_MAILBOX)).toBeVisible()
        await expect(addressView(page, secondAddress)).toBeVisible()

        const { subject } = await deliverInbound(request, {
            subject: uniqueSubject('Address view'),
            to: secondAddress,
        })
        // A message to the main address, so the main view below has a row to
        // show and the missing row is an answer, not a list still loading.
        const { subject: mainSubject } = await deliverInbound(request, {
            subject: uniqueSubject('Main address view'),
        })

        await addressView(page, secondAddress).click()
        await expectRowVisible(page, subject)
        await expect(emailRow(page, mainSubject)).toBeHidden()

        await addressView(page, TEST_USER_MAILBOX).click()
        await expectRowVisible(page, mainSubject)
        await expect(emailRow(page, subject)).toBeHidden()

        await page.getByText('Inbox', { exact: true }).first().click()
        await expectRowVisible(page, subject)
        await expectRowVisible(page, mainSubject)

        await setAddressViews(page, false)
        await expect(page.getByText('Show in sidebar', { exact: true })).toBeHidden()
        await openMail(page)
        // The marker shows once the views query has loaded, so no entries
        // below is an answer and not a sidebar still loading.
        await page.getByTestId('address-views-ready').waitFor({ state: 'attached' })
        await expect(page.locator('[data-testid^="address-view-"]')).toHaveCount(0)
    })
})
