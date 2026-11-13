import { expect, type Locator, type Page, test } from '@playwright/test'
import { login, navigateToPackage } from '@tinycld/core/e2e-helpers'
import PocketBase from 'pocketbase'
import {
    deliverInbound,
    emailRow,
    expectRowVisible,
    navigateToPersonalInbox,
    openThread,
    PB_URL,
    SUPERUSER_EMAIL,
    SUPERUSER_PASSWORD,
    TEST_USER_USERNAME,
    uniqueSubject,
} from './helpers'

// AI classification, end to end through the UI. The e2e server is built with
// the `classifierfake` tag: an in-process stand-in for the model accepts the key
// "test-key" and picks the label named in a `[jev:<label>]` marker in the
// subject. A subject without a marker is refused, so mail from other specs is
// never labelled.
//
// One fixture is seeded by a superuser write: the sent-mail record behind "I
// have sent email to the sender". The real path is a send, and the e2e server
// has no mail provider credentials, so a send cannot succeed and never records
// the recipient. The spec therefore does not cover the send that records a
// correspondent (server/correspondents_test.go covers it); it covers only how a
// rule reads that record.

const PHONE = { width: 390, height: 844 }
const USER_PREFERENCE_WRITE = /\/api\/collections\/user_preferences\/records/

// Settings entries share labels across the core system panel, the mail
// organization panel and the mail account panel, so they are picked by testID.
async function openSettingsLink(page: Page, testID: string) {
    await page.getByTestId('nav-settings').click()
    await page.getByTestId('settings-index').waitFor({ state: 'visible' })
    await page.getByTestId(testID).click()
}

async function enableClassifier(page: Page) {
    await openSettingsLink(page, 'settings-system-ai-classification')
    await page.getByTestId('classifier-api-key').fill('test-key')
    const save = page.getByTestId('classifier-save')
    await expect(save).toBeEnabled()
    await save.click()
    // The form resets to clean only after every setting is written.
    await expect(save).toBeDisabled()
    await page.getByTestId('classifier-verify').click()
    await expect(page.getByTestId('classifier-verify-result')).toHaveText('Connected')
}

async function ensureUniversalLabels(page: Page) {
    await openSettingsLink(page, 'settings-org-mail-classification')
    const editor = 'mail-classifier-org-editor'
    // Enabled once the stored taxonomy has loaded.
    await expect(page.getByTestId(`${editor}-save`)).toBeEnabled()
    const hasLabels = (await page.getByTestId(`${editor}-row-0-name`).count()) > 0
    if (hasLabels) return

    await page.getByTestId(`${editor}-preset`).click()
    await page.getByTestId(`${editor}-preset-universal`).click()
    // The last label written is "review"; its rule text is unique to it.
    const lastWrite = page.waitForResponse(
        res =>
            res.request().method() === 'POST' &&
            res.url().includes('/api/collections/classifier_labels/records') &&
            (res.request().postData() ?? '').includes('does not clearly fit')
    )
    await page.getByTestId(`${editor}-save`).click()
    await lastWrite
}

let modeChanged = false

async function setMode(page: Page, mode: 'off' | 'preview' | 'labels' | 'labels_archive') {
    await openSettingsLink(page, 'settings-account-mail-classification')
    modeChanged = mode !== 'off'
    await page.getByTestId('mail-classifier-mode').click()
    const written = page.waitForResponse(
        res => USER_PREFERENCE_WRITE.test(res.url()) && res.request().method() !== 'GET'
    )
    await page.getByTestId(`mail-classifier-mode-${mode}`).click()
    await written
}

function recentRow(page: Page, subject: string): Locator {
    return page
        .locator('[data-testid="mail-classifier-recent-row"]:visible')
        .filter({ hasText: subject })
}

async function goToInbox(page: Page) {
    await navigateToPackage(page, 'mail', { waitFor: page.getByTestId('package-sidebar-mounted') })
    await navigateToPersonalInbox(page)
}

async function selectFromMenu(page: Page, trigger: Locator, optionLabel: string) {
    await trigger.click()
    await page.getByText(optionLabel, { exact: true }).click()
}

// A personal rule that stars the message, built through the mail Rules screen.
async function buildStarRule(
    page: Page,
    opts: { name: string; trigger: string; fieldTestID: string; value?: string }
) {
    await navigateToPackage(page, 'mail', { waitFor: page.getByTestId('package-sidebar-mounted') })
    await page.getByTestId('mail-sidebar-rules').click()
    await expect(page.getByText('Mail rules', { exact: true })).toBeVisible()

    await page.getByText('New rule', { exact: true }).first().click()
    await expect(page.getByText('New rule', { exact: true }).last()).toBeVisible()
    await page.getByPlaceholder('Rule name').fill(opts.name)
    await selectFromMenu(page, page.getByText('Select a trigger…', { exact: true }), opts.trigger)

    await page.getByText('Field…', { exact: true }).click()
    await page.getByTestId(opts.fieldTestID).click()
    if (opts.value) await page.getByRole('textbox').last().fill(opts.value)

    await page.getByText('add action', { exact: true }).click()
    await page.getByText('Star the message', { exact: true }).click()
    // Earlier screens stay mounted, so scope Save to the builder that holds the
    // rule name field.
    const builder = page
        .locator('div')
        .filter({ has: page.getByPlaceholder('Rule name') })
        .filter({ has: page.getByText('Save', { exact: true }) })
        .last()
    await builder.getByText('Save', { exact: true }).click()
    await expect(page.getByText(opts.name, { exact: true })).toBeVisible()
}

async function seedCorrespondent(email: string) {
    const pb = new PocketBase(PB_URL)
    await pb.collection('_superusers').authWithPassword(SUPERUSER_EMAIL, SUPERUSER_PASSWORD)
    const user = await pb.collection('users').getFirstListItem(`username = "${TEST_USER_USERNAME}"`)
    const filter = pb.filter('user = {:user} && email = {:email}', { user: user.id, email })
    const existing = await pb.collection('mail_correspondents').getList(1, 1, { filter })
    if (existing.totalItems > 0) return
    await pb.collection('mail_correspondents').create({
        user: user.id,
        email,
        last_sent: new Date().toISOString(),
    })
}

test.describe('AI classification', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await enableClassifier(page)
        await ensureUniversalLabels(page)
    })

    test.afterEach(async ({ page }) => {
        if (!modeChanged) return
        await setMode(page, 'off')
    })

    test('labels a new message and shows the AI chip', async ({ page, request }) => {
        await setMode(page, 'labels')
        // The seed already has a "Newsletters" label; the taxonomy reuses a
        // label by name, so the model sees that exact spelling.
        const { subject } = await deliverInbound(request, {
            subject: uniqueSubject('[jev:Newsletters] Weekly'),
        })
        await goToInbox(page)
        await openThread(page, subject)
        await expect(page.getByTestId('mail-ai-chip')).toContainText('Newsletters')
    })

    test('archives a new junk thread but not a reply', async ({ page, request }) => {
        await setMode(page, 'labels_archive')
        const fresh = await deliverInbound(request, { subject: uniqueSubject('[jev:junk] Prize') })
        await expect(recentRow(page, fresh.subject)).toContainText('Labelled and archived')
        await goToInbox(page)
        await expect(emailRow(page, fresh.subject)).toHaveCount(0)

        // The base message has no marker, so it is never labelled; the reply
        // joins its thread by In-Reply-To, so the thread already has a message.
        const base = await deliverInbound(request, { subject: uniqueSubject('Thread start') })
        await expectRowVisible(page, base.subject)
        const reply = await deliverInbound(request, {
            subject: `Re: ${base.subject} [jev:junk]`,
            inReplyTo: base.messageId,
        })
        await openSettingsLink(page, 'settings-account-mail-classification')
        const row = recentRow(page, reply.subject)
        await expect(row).toContainText('Labelled')
        await expect(row).not.toContainText('archived')
        await goToInbox(page)
        await expectRowVisible(page, base.subject)
    })

    test('preview mode records a result without labelling', async ({ page, request }) => {
        await setMode(page, 'preview')
        const { subject } = await deliverInbound(request, {
            subject: uniqueSubject('[jev:personal] Hi'),
        })
        const recent = page.getByTestId('mail-classifier-recent')
        await expect(recent).toContainText(subject)
        await expect(recent).toContainText('Not changed (preview)')
        await goToInbox(page)
        await openThread(page, subject)
        await expect(page.getByTestId('mail-ai-chip')).toHaveCount(0)
    })

    test('a message-classified rule runs', async ({ page, request }) => {
        await setMode(page, 'labels')
        await buildStarRule(page, {
            name: `AI opportunity rule ${Date.now()}`,
            trigger: 'AI classifies a message',
            fieldTestID: 'condition-field-option-label_name',
            value: 'opportunities',
        })
        const { subject } = await deliverInbound(request, {
            subject: uniqueSubject('[jev:opportunities] Partnership'),
        })
        await page.getByText('Inbox', { exact: true }).first().click()
        await expectRowVisible(page, subject)
        await expect(
            emailRow(page, subject).getByRole('button', { name: 'Remove star', exact: true })
        ).toBeVisible()
    })

    test('a has-sent-to rule on message-received runs', async ({ page, request }) => {
        await seedCorrespondent('known@example.com')
        await buildStarRule(page, {
            name: `Known sender rule ${Date.now()}`,
            trigger: 'A message arrives',
            fieldTestID: 'condition-field-option-fields.has_sent_to_sender',
        })
        const { subject } = await deliverInbound(request, {
            subject: uniqueSubject('From a known sender'),
            fromEmail: 'known@example.com',
        })
        await page.getByText('Inbox', { exact: true }).first().click()
        await expectRowVisible(page, subject)
        await expect(
            emailRow(page, subject).getByRole('button', { name: 'Remove star', exact: true })
        ).toBeVisible()
    })
})

test.describe('AI classification — phone', () => {
    test('account settings at phone size', async ({ page }) => {
        await login(page)
        await enableClassifier(page)
        // The phone tab bar has no direct settings link, so reach the settings
        // index at the default size, then shrink and open the panel.
        await page.getByTestId('nav-settings').click()
        await page.getByTestId('settings-index').waitFor({ state: 'visible' })
        await page.setViewportSize(PHONE)
        await page.getByTestId('settings-account-mail-classification').click()
        await expect(page.getByTestId('mail-classifier-mode')).toBeVisible()
        await expect(page.getByTestId('mail-classifier-recent')).toBeVisible()
    })
})
