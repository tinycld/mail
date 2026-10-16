// mail-person-email.spec.ts — an admin manages a person's email from Members.
//
// A second domain is seeded VERIFIED by a superuser write. Verification needs
// a live DNS MX lookup and provider enrollment, which CI does not have, so
// the add-and-verify path cannot run here (mail-custom-domain.spec.ts stubs
// the add endpoint for the same reason). The second domain is needed because
// on tinycld.org the person's address is the inbox's main address, which a
// rename keeps; only a rule address is lost and kept by "Keep the old
// addresses". This spec therefore does NOT cover adding or verifying a domain.
// Everything after the seed — the invite, the domain rule, the added address,
// the Default From and the rename — is driven through the UI.
import { expect, type Locator, test } from '@playwright/test'
import { createInvitedUser } from '@tinycld/core/e2e-helpers'
import {
    giveEveryoneAnAddressOn,
    navigateToSharedInboxSettings,
    openPersonEmail,
    seedVerifiedDomain,
} from './helpers'

const MAIN_DOMAIN = 'tinycld.org'

// The row, not the bare text: the Default From choices list each address too.
function addressRow(drawer: Locator, email: string) {
    return drawer.getByTestId(`address-row-${email}`)
}

function defaultFromChoice(drawer: Locator, email: string) {
    return drawer
        .getByRole('radiogroup', { name: 'Send new mail from' })
        .getByRole('radio', { name: email, exact: true })
}

test.describe('Mail — A person’s email in Members', () => {
    test('an admin adds an address, sets the Default From and renames the email name', async ({
        page,
    }) => {
        const stamp = `${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
        const ruleDomain = `person-${stamp}.example.com`
        await seedVerifiedDomain(ruleDomain)

        const { user, close } = await createInvitedUser(page, 'mailp')
        await close()
        await giveEveryoneAnAddressOn(page, ruleDomain)

        const mainEmail = `${user.username}@${MAIN_DOMAIN}`
        const oldRuleEmail = `${user.username}@${ruleDomain}`
        const addedEmail = `p${stamp}@${MAIN_DOMAIN}`
        const newMainDomainEmail = `r${stamp}@${MAIN_DOMAIN}`
        const newRuleEmail = `r${stamp}@${ruleDomain}`

        const drawer = await openPersonEmail(page, user.username)
        const mainRow = addressRow(drawer, mainEmail)
        await expect(mainRow.getByText('Main address', { exact: true })).toBeVisible()
        const oldRuleRow = addressRow(drawer, oldRuleEmail)
        await expect(oldRuleRow.getByText('From the domain rule', { exact: true })).toBeVisible()

        await drawer.getByText('Add address', { exact: true }).click()
        await drawer.getByTestId('local_part').fill(`p${stamp}`)
        // Other specs add domains while this one runs, so pick the domain by name.
        await drawer
            .getByRole('radiogroup', { name: 'Domain' })
            .getByRole('radio', { name: MAIN_DOMAIN, exact: true })
            .click()
        await drawer.getByText('Add address', { exact: true }).click()
        const addedRow = addressRow(drawer, addedEmail)
        await expect(addedRow).toBeVisible()
        // The row holds only the address: no badge.
        await expect(addedRow).toHaveText(addedEmail)
        await expect(
            addedRow.getByRole('button', { name: `Remove address ${addedEmail}` })
        ).toBeVisible()

        await defaultFromChoice(drawer, addedEmail).click()
        await expect(defaultFromChoice(drawer, addedEmail)).toHaveAttribute('aria-checked', 'true')

        await drawer.getByText('Edit', { exact: true }).click()
        await drawer.getByTestId('email_name').fill(`r${stamp}`)
        await expect(
            drawer.getByRole('switch', { name: 'Keep the old addresses as extra addresses' })
        ).toHaveAttribute('aria-checked', 'true')
        await drawer.getByText('Rename', { exact: true }).click()
        await expect(drawer.getByText('Rename', { exact: true })).toBeHidden()

        for (const email of [newMainDomainEmail, newRuleEmail]) {
            await expect(
                addressRow(drawer, email).getByText('From the domain rule', { exact: true })
            ).toBeVisible()
        }
        // The rename removed the old rule address; "Keep the old addresses"
        // added it back as an address that an admin can remove.
        await expect(oldRuleRow).toHaveText(oldRuleEmail)
        await expect(
            oldRuleRow.getByRole('button', { name: `Remove address ${oldRuleEmail}` })
        ).toBeVisible()
        await expect(mainRow.getByText('Main address', { exact: true })).toBeVisible()
        await expect(defaultFromChoice(drawer, addedEmail)).toHaveAttribute('aria-checked', 'true')

        // The drawer is modal: its backdrop covers the app rail until it closes.
        await drawer.getByRole('button', { name: 'Close' }).click()
        await expect(drawer).toBeHidden()

        await navigateToSharedInboxSettings(page)
        // The list footer or the empty state proves the inbox list loaded, so
        // a missing row below is an answer and not a list still loading.
        await expect(
            page
                .getByText(/^\d+ shared inbox/)
                .or(page.getByText('No shared inboxes yet', { exact: true }))
                .first()
        ).toBeVisible()
        await expect(page.locator(`[data-testid^="mailbox-row-${user.username}@"]`)).toHaveCount(0)
    })
})
