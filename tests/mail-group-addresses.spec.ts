// mail-group-addresses.spec.ts — a group's own address sends mail to each member.
//
// Everything runs on the main domain (tinycld.org) and is driven through the
// UI: the invite, the group and its members, the group's address, and the
// removal from the group. Only the inbound messages come in through the
// provider webhook, as real mail does. The invitee keeps a page open, so the
// second message staying out of that inbox is checked on a live list, after
// a later marker message to the invitee's own address has arrived.
import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, navigateToPackage, TEST_USER_EMAIL } from '@tinycld/core/e2e-helpers'
import {
    deliverInbound,
    emailRow,
    expectRowVisible,
    navigateToPersonalInbox,
    uniqueSubject,
} from './helpers'

const MAIN_DOMAIN = 'tinycld.org'

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

function confirmDialog(page: Page) {
    return page.getByRole('dialog').filter({ has: page.getByTestId('confirm-dialog-body') })
}

async function addMember(page: Page, email: string) {
    const drawer = page.getByRole('dialog')
    await drawer.getByTestId('group-add-member-search').fill(email)
    await drawer.getByTestId(`group-candidate-row-${email}`).getByRole('button').click()
    await expect(drawer.getByTestId(`group-member-row-${email}`)).toBeVisible()
}

async function addGroupAddress(page: Page, localPart: string) {
    const drawer = page.getByRole('dialog')
    await drawer.getByText('Add address', { exact: true }).click()
    await drawer.getByTestId('local_part').fill(localPart)
    // Other specs add domains while this one runs, so pick the domain by name.
    await drawer
        .getByRole('radiogroup', { name: 'Domain' })
        .getByRole('radio', { name: MAIN_DOMAIN, exact: true })
        .click()
    await drawer.getByText('Add address', { exact: true }).click()
    await expect(drawer.getByTestId(`group-address-${localPart}@${MAIN_DOMAIN}`)).toBeVisible()
}

async function openMailInbox(page: Page) {
    await navigateToPackage(page, 'mail', {
        waitFor: page.getByTestId('package-sidebar-mounted'),
    })
    await navigateToPersonalInbox(page)
}

// The cleanup runs after a failed step too: it first closes a confirm or a
// drawer that the failed step left open, because both cover the app rail.
async function deleteGroup(page: Page, groupName: string) {
    const confirm = confirmDialog(page)
    if (await confirm.isVisible()) {
        await confirm.getByRole('button', { name: 'Cancel', exact: true }).click()
        await expect(confirm).toBeHidden()
    }
    const drawer = page.getByRole('dialog').first()
    if (await drawer.isVisible()) {
        await drawer.getByRole('button', { name: 'Close' }).click()
        await expect(drawer).toBeHidden()
    }

    await openGroupsSettings(page)
    const row = page.getByTestId(`group-row-${groupName}`)
    await row.click()
    await page.getByRole('dialog').getByTestId('group-delete').click()
    await confirmDialog(page).getByRole('button', { name: 'Delete group', exact: true }).click()
    await expect(confirmDialog(page)).toBeHidden()
    await expect(row).toBeHidden()
}

test.describe('Mail — A group’s own address', () => {
    const stamp = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
    const groupName = `List ${stamp}`
    const localPart = `list${stamp}`
    const groupAddress = `${localPart}@${MAIN_DOMAIN}`
    let isGroupCreated = false
    let closeInvitee: (() => Promise<void>) | null = null

    test.afterEach(async ({ page }) => {
        try {
            if (isGroupCreated) await deleteGroup(page, groupName)
        } finally {
            if (closeInvitee) await closeInvitee()
        }
    })

    test('mail to the group address reaches each member, and stops for a removed member', async ({
        page,
        request,
    }) => {
        const { user, inviteePage, close } = await createInvitedUser(page, 'grpaddr')
        // createInvitedUser leaves the fixture admin signed in on this page.
        closeInvitee = close

        await openGroupsSettings(page)
        await page.getByTestId('groups-new-button').click()
        const drawer = page.getByRole('dialog')
        await drawer.getByTestId('name').fill(groupName)
        await drawer.getByTestId('group-create-submit').click()
        await expect(drawer.getByText('Members and details', { exact: true })).toBeVisible()
        isGroupCreated = true
        await addMember(page, TEST_USER_EMAIL)
        await addMember(page, user.email)
        await addGroupAddress(page, localPart)
        await closeDrawer(page)

        const { subject: first } = await deliverInbound(request, {
            subject: uniqueSubject('Group address'),
            to: groupAddress,
        })

        await openMailInbox(page)
        await expectRowVisible(page, first)

        await openMailInbox(inviteePage)
        await expectRowVisible(inviteePage, first)

        await openGroupsSettings(page)
        await page.getByTestId(`group-row-${groupName}`).click()
        const inviteeRow = page.getByTestId(`group-member-row-${user.email}`)
        await inviteeRow.getByRole('button').click()
        const confirm = confirmDialog(page)
        await expect(confirm.getByText(`stops getting mail sent to ${groupAddress}.`)).toBeVisible()
        await confirm.getByRole('button', { name: 'Remove', exact: true }).click()
        await expect(confirm).toBeHidden()
        await expect(inviteeRow).toBeHidden()
        await closeDrawer(page)

        const { subject: second } = await deliverInbound(request, {
            subject: uniqueSubject('Group address after removal'),
            to: groupAddress,
        })

        // The webhook stores every copy before it answers, so the fixture
        // user's copy proves the invitee's copy would already exist.
        await openMailInbox(page)
        await expectRowVisible(page, second)

        // The invitee's list is live, so an absence check alone could pass
        // before a copy of the second message arrives. A marker sent to the
        // invitee's own address after the second message arrives after any
        // copy of it would, so once the marker shows, the second is absent
        // for good.
        const { subject: marker } = await deliverInbound(request, {
            subject: uniqueSubject('Marker after removal'),
            to: `${user.username}@${MAIN_DOMAIN}`,
        })
        await expectRowVisible(inviteePage, marker)
        await expect(emailRow(inviteePage, second)).toHaveCount(0)
    })
})
