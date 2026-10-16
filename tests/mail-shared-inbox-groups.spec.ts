// mail-shared-inbox-groups.spec.ts — a group gives its people a shared inbox.
//
// Everything is driven through the UI: the invite, the group and its member,
// the shared inbox and its group grant, and the removal from the group. The
// invitee keeps a page open the whole time, so the inbox going away after the
// removal proves that realtime reaches the invitee's sidebar without a reload.
import { expect, type Page, test } from '@playwright/test'
import { createInvitedUser, navigateToPackage } from '@tinycld/core/e2e-helpers'
import { createSharedMailbox, mailboxSection, navigateToSharedInboxSettings } from './helpers'

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

async function createGroupWithMember(page: Page, groupName: string, memberEmail: string) {
    await openGroupsSettings(page)
    await page.getByTestId('groups-new-button').click()
    const drawer = page.getByRole('dialog')
    await drawer.getByTestId('name').fill(groupName)
    await drawer.getByTestId('group-create-submit').click()
    await expect(drawer.getByText('Members and details', { exact: true })).toBeVisible()
    await drawer.getByTestId('group-add-member-search').fill(memberEmail)
    await drawer.getByTestId(`group-candidate-row-${memberEmail}`).getByRole('button').click()
    await expect(drawer.getByTestId(`group-member-row-${memberEmail}`)).toBeVisible()
    await closeDrawer(page)
}

test.describe('Mail — Groups in shared inboxes', () => {
    test('a group gives its people the inbox, and a person loses it when removed from the group', async ({
        page,
    }) => {
        const stamp = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
        const groupName = `Inbox G ${stamp}`

        const { user, inviteePage, close } = await createInvitedUser(page, 'inboxg')
        try {
            await createGroupWithMember(page, groupName, user.email)

            const inboxAddress = await createSharedMailbox(page, 'grp')
            const local = inboxAddress.split('@')[0]

            await navigateToSharedInboxSettings(page)
            await page.getByTestId(`mailbox-row-${inboxAddress}`).click()
            const drawer = page.getByRole('dialog')
            const groups = drawer.getByTestId('group-share-section')
            await groups.getByText('Add group', { exact: true }).click()
            const picker = page.getByRole('dialog').filter({ hasText: 'Add a group' })
            await picker.getByTestId('group-picker-search').fill(groupName)
            await picker
                .locator('[data-testid^="group-picker-row-"]')
                .filter({ hasText: groupName })
                .getByText('Add', { exact: true })
                .click()
            await expect(picker).toBeHidden()
            await expect(groups.getByText(groupName, { exact: true })).toBeVisible()

            // The nearest ancestor holding the invitee's email and the group
            // note, so the note cannot belong to another member's row.
            const inviteeRow = drawer
                .locator('div')
                .filter({ has: page.getByText(user.email, { exact: true }) })
                .filter({ has: page.getByText(`Through group ${groupName}`, { exact: true }) })
                .last()
            await expect(inviteeRow).toBeVisible()
            await closeDrawer(page)

            await navigateToPackage(inviteePage, 'mail', {
                waitFor: inviteePage.getByTestId('package-sidebar-mounted'),
            })
            await expect(mailboxSection(inviteePage, local)).toBeVisible()

            await openGroupsSettings(page)
            await page.getByTestId(`group-row-${groupName}`).click()
            const memberRow = page.getByTestId(`group-member-row-${user.email}`)
            await memberRow.getByRole('button').click()
            const confirm = page
                .getByRole('dialog')
                .filter({ has: page.getByTestId('confirm-dialog-body') })
            await expect(confirm.getByText(`loses access to ${inboxAddress}.`)).toBeVisible()
            await confirm.getByRole('button', { name: 'Remove', exact: true }).click()
            await expect(confirm).toBeHidden()
            await expect(memberRow).toBeHidden()
            await closeDrawer(page)

            // No reload: the derived membership row's delete reaches the
            // invitee's open sidebar through realtime.
            await expect(mailboxSection(inviteePage, local)).toHaveCount(0)
        } finally {
            await close()
        }
    })
})
