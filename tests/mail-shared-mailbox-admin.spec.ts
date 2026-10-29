import { expect, test } from '@playwright/test'
import PocketBase from 'pocketbase'
import {
    navigateToSharedInboxSettings,
    PB_URL,
    SUPERUSER_EMAIL,
    SUPERUSER_PASSWORD,
} from './helpers'

interface SeededUser {
    email: string
    username: string
    password: string
    name: string
}

// Seeds a test account with a superuser write, taking the documented
// exception to "e2e never writes data directly" (see CLAUDE.md).
//
// WHY the real path is unreachable: creating a user through the UI requires
// the invite flow, which sends a real invitation email and needs the
// recipient to open a link from an inbox. CI has no mail provider
// credentials and no second inbox to read, so no admin+member pair can be
// brought into existence by driving the app.
//
// WHAT this therefore does NOT cover: none of user creation is exercised
// here — not the invite endpoint, not the invitation email, not the accept
// flow, and not the role assignment the invite form performs. Do not read
// this spec as end-to-end coverage of provisioning a user; it covers only
// what an already-existing admin and member can each see and do on the
// shared-mailbox settings screen.
//
// Single-org: the role lives on the users record itself, so seeding a test
// account is a single create (the former orgs + user_org junction is gone).
async function seedUser(role: 'admin' | 'member', label: string): Promise<SeededUser> {
    const pb = new PocketBase(PB_URL)
    await pb.collection('_superusers').authWithPassword(SUPERUSER_EMAIL, SUPERUSER_PASSWORD)

    const stamp = `${Date.now().toString(36)}-${Math.floor(Math.random() * 1e6).toString(36)}`
    const seeded: SeededUser = {
        email: `${label}-${stamp}@example.com`,
        username: `${label}${stamp.replace(/-/g, '')}`,
        password: 'TestPass1234!',
        name: `${label.charAt(0).toUpperCase() + label.slice(1)} Tester`,
    }

    await pb.collection('users').create({
        email: seeded.email,
        username: seeded.username,
        password: seeded.password,
        passwordConfirm: seeded.password,
        name: seeded.name,
        emailVisibility: true,
        verified: true,
        role,
    })

    return seeded
}

async function loginAs(page: import('@playwright/test').Page, user: SeededUser) {
    await page.goto('/')
    await page.getByTestId('identifier').fill(user.username)
    await page.getByPlaceholder('Password').fill(user.password)
    await page.getByText('Sign in', { exact: true }).last().click()
    await expect(page.getByTestId('nav-home')).toBeVisible({ timeout: 20_000 })
}

test.describe('Mail — Shared mailbox lifecycle as admin', () => {
    test('creating a shared mailbox with a duplicate address surfaces a form error', async ({
        page,
    }) => {
        const admin = await seedUser('admin', 'dupadmin')

        await loginAs(page, admin)
        await navigateToSharedInboxSettings(page)

        // Create a baseline mailbox owned by this test, then attempt to
        // create a second one with the same address. The form knows every
        // existing address, so it refuses the duplicate before it is sent.
        // We don't reuse the seeded "support" mailbox — other parallel specs
        // may delete/rename it.
        const address = `dup-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`

        await page.getByText('New shared inbox', { exact: true }).click()
        await expect(page.getByText('Set up a shared inbox')).toBeVisible()
        await page.getByTestId('address').fill(address)
        await page.getByTestId('display_name').fill('Original')
        await page.getByText('Create inbox', { exact: true }).click()
        await expect(page.getByText('Set up a shared inbox')).toBeHidden()

        // Now the duplicate attempt.
        await page.getByText('New shared inbox', { exact: true }).click()
        await expect(page.getByText('Set up a shared inbox')).toBeVisible()
        await page.getByTestId('address').fill(address)
        await page.getByTestId('display_name').fill('Duplicate Test')

        // The address field names the full address that collides; the
        // random address keeps the match from hitting other content.
        await expect(page.getByText(new RegExp(`^${address}@\\S+ already exists$`))).toBeVisible()
        await expect(page.getByTestId('mailbox-form-submit')).toBeDisabled()
    })

    test('admin can create a shared mailbox and add another org member to it', async ({ page }) => {
        const admin = await seedUser('admin', 'admin')
        const teammate = await seedUser('member', 'mate')

        await loginAs(page, admin)
        await navigateToSharedInboxSettings(page)

        const address = `admin-${Date.now().toString(36)}`

        await page.getByText('New shared inbox', { exact: true }).click()
        await expect(page.getByText('Set up a shared inbox')).toBeVisible()
        await page.getByTestId('address').fill(address)
        await page.getByTestId('display_name').fill('Admin Test')
        await page.getByText('Create inbox', { exact: true }).click()

        // Drawer closes only when both inserts (mailbox + first owner member) succeed.
        await expect(page.getByText('Set up a shared inbox')).toBeHidden()
        await expect(page.getByText(`Admin Test · 1 member`)).toBeVisible()

        // Open the mailbox in view mode to access the Members tab.
        await page.getByText(`Admin Test · 1 member`).click()
        await expect(page.getByText('Who has access')).toBeVisible()

        // Add the teammate as a member of the mailbox.
        await page.getByText('Add member', { exact: true }).click()
        await page.getByText(teammate.name, { exact: true }).click()
        await page.getByText('Add', { exact: true }).last().click()

        // The drawer's Members tab shows the count; the row for the new teammate
        // appears in the "Who has access" list. Both update only when the
        // mail_mailbox_members create rule lets the owner add another user.
        await expect(page.getByText('Members · 2')).toBeVisible()
        await expect(page.getByText(teammate.name).first()).toBeVisible()
        await expect(page.getByText(teammate.email).first()).toBeVisible()
    })
})
