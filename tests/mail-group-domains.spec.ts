// mail-group-domains.spec.ts — a domain gives addresses to members of groups.
//
// The domain is seeded VERIFIED by a superuser write. Verification
// needs a live DNS MX lookup and provider enrollment, which CI does not have,
// so the add-and-verify path cannot run here (mail-custom-domain.spec.ts
// stubs the add endpoint for the same reason).
// This spec therefore does NOT cover adding or verifying a domain.
// Everything after the seed — the group and its member, the domain's rule,
// the address in the mailbox, the removal notice and the address going
// away — is driven through the UI.
import { expect, type Page, test } from '@playwright/test'
import { login, navigateToPackage, TEST_USER_EMAIL } from '@tinycld/core/e2e-helpers'
import PocketBase from 'pocketbase'
import { navigateToMailboxSettings, TEST_USER_MAILBOX } from './helpers'

// PB sits behind the dev.ts proxy on the test Expo port. /api/* routes
// through to PB transparently — see scripts/dev.ts::isPbPath.
const PB_URL = 'http://127.0.0.1:7200'
const SUPERUSER_EMAIL = process.env.POCKETBASE_EMAIL || 'admin@tinycld.org'
const SUPERUSER_PASSWORD = process.env.POCKETBASE_PASSWORD || 'AdminPass1234!'

async function seedVerifiedDomain(domain: string) {
    const pb = new PocketBase(PB_URL)
    await pb.collection('_superusers').authWithPassword(SUPERUSER_EMAIL, SUPERUSER_PASSWORD)
    await pb.collection('mail_domains').create({
        domain,
        verified: true,
        mx_verified: true,
        inbound_domain_verified: true,
        spf_verified: true,
        dkim_verified: true,
        return_path_verified: true,
    })
}

async function openGroupsSettings(page: Page) {
    await page.getByTestId('nav-settings').click()
    await page.getByText('Groups', { exact: true }).first().click()
    await expect(page.getByTestId('groups-new-button')).toBeVisible()
}

// The drawer is modal: its backdrop covers the app rail until it closes.
async function closeDrawer(page: Page) {
    const drawer = page.getByRole('dialog')
    await drawer.getByRole('button', { name: 'Close' }).click()
    await expect(drawer).toBeHidden()
}

async function openTestUserMailbox(page: Page) {
    await navigateToMailboxSettings(page)
    await page.getByTestId(`mailbox-row-${TEST_USER_MAILBOX}`).click()
    const drawer = page.getByRole('dialog')
    await expect(drawer.getByText('Addresses', { exact: true })).toBeVisible()
    // The main address proves the address list loaded, so a missing address
    // below is an answer and not a list that has not rendered yet.
    await expect(drawer.getByText(TEST_USER_MAILBOX, { exact: true }).first()).toBeVisible()
    return drawer
}

test.describe('Mail — Domains for members of groups', () => {
    test('a group member gets an address, and loses it when removed from the group', async ({
        page,
    }) => {
        const stamp = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
        const domain = `groups-${stamp}.example.com`
        const groupName = `Q staff ${stamp}`
        const localPart = TEST_USER_MAILBOX.split('@')[0]
        const address = `${localPart}@${domain}`

        await seedVerifiedDomain(domain)

        await login(page)

        await openGroupsSettings(page)
        await page.getByTestId('groups-new-button').click()
        const groupDrawer = page.getByRole('dialog')
        await groupDrawer.getByTestId('name').fill(groupName)
        await groupDrawer.getByTestId('group-create-submit').click()
        await expect(groupDrawer.getByText('Members and details', { exact: true })).toBeVisible()
        await groupDrawer.getByTestId('group-add-member-search').fill(TEST_USER_EMAIL)
        await groupDrawer
            .getByTestId(`group-candidate-row-${TEST_USER_EMAIL}`)
            .getByRole('button')
            .click()
        await expect(groupDrawer.getByTestId(`group-member-row-${TEST_USER_EMAIL}`)).toBeVisible()
        await closeDrawer(page)

        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
        await page.getByTestId('nav-settings').click()
        await page.getByText('Domains', { exact: true }).first().click()
        await expect(page.getByText('Mail Domains', { exact: true })).toBeVisible()

        // The nearest ancestor holding this domain's name and its rule control,
        // so a click cannot land in another domain's card.
        const card = page
            .locator('div')
            .filter({ has: page.getByText(domain, { exact: true }) })
            .filter({ has: page.getByText('Who gets an address here?', { exact: true }) })
            .last()
        await expect(card.getByText('Only people I add', { exact: true })).toBeVisible()

        await card.getByText('Members of these groups', { exact: true }).click()
        await card.getByRole('checkbox', { name: groupName, exact: true }).click()
        await expect(card.getByText(`Adds 1: ${address}`, { exact: true })).toBeVisible()
        await card.getByText('Apply', { exact: true }).click()
        await expect(card.getByText('Apply', { exact: true })).toBeHidden()

        const mailboxDrawer = await openTestUserMailbox(page)
        await expect(mailboxDrawer.getByText(address, { exact: true })).toBeVisible()
        await expect(
            mailboxDrawer.getByText(`From group ${groupName}`, { exact: true })
        ).toBeVisible()
        await closeDrawer(page)

        await openGroupsSettings(page)
        await page.getByTestId(`group-row-${groupName}`).click()
        const memberRow = page.getByTestId(`group-member-row-${TEST_USER_EMAIL}`)
        await memberRow.getByRole('button').click()
        const confirm = page
            .getByRole('dialog')
            .filter({ has: page.getByTestId('confirm-dialog-body') })
        await expect(
            confirm.getByText('They lose access to everything this group gives them.')
        ).toBeVisible()
        await expect(confirm.getByText(`loses the address ${address}.`)).toBeVisible()
        await confirm.getByRole('button', { name: 'Remove', exact: true }).click()
        await expect(confirm).toBeHidden()
        await expect(memberRow).toBeHidden()
        await closeDrawer(page)

        const reopenedDrawer = await openTestUserMailbox(page)
        await expect(reopenedDrawer.getByText(address, { exact: true })).toBeHidden()
        await closeDrawer(page)
    })
})
