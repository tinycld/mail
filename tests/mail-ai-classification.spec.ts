import { expect, type Locator, type Page, test } from '@playwright/test'
import { clickSidebarItem, login, navigateToPackage } from '@tinycld/core/e2e-helpers'
import {
    deliverInbound,
    emailRow,
    expectRowVisible,
    navigateToPersonalInbox,
    openThread,
    TEST_USER_MAILBOX,
    uniqueSubject,
} from './helpers'
import { appendMessage, findPersonalInbox, listMailboxes, withImapClient } from './imap-helpers'

// AI classification, end to end through the UI. The e2e server is built with
// the `classifierfake` tag: an in-process stand-in for the model accepts the key
// "test-key" and picks the label named in a `[jev:<label>]` marker in the
// subject. A subject without a marker is refused, so mail from other specs is
// never labelled.
//
// The compose UI cannot send in e2e (there is no mail provider), so the "I have
// sent email to the sender" record comes from an IMAP client saving a message
// to Sent, which the server records as a sent message to that address.
//
// The spec leaves the system classifier key and the org taxonomy set. That is
// harmless to other specs: each user's mode defaults to off, and the fake
// refuses subjects without a marker.

const PHONE = { width: 390, height: 844 }
const USER_PREFERENCE_WRITE = /\/api\/collections\/user_preferences\/records/
const ORG_EDITOR = 'mail-classifier-org-editor'

type Mode = 'off' | 'preview' | 'labels' | 'labels_archive'

const MODE_LABELS: Record<Mode, string> = {
    off: 'Off',
    preview: 'Preview only',
    labels: 'Labels',
    labels_archive: 'Labels and archive',
}

// Names the tests below mark subjects with. The taxonomy owns its option
// names (the model sees them, not the core label's), so they are the preset's.
const NEEDED_LABELS = ['junk', 'opportunities', 'newsletters', 'personal']

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

async function orgLabelNames(page: Page) {
    const inputs = page.locator(`[data-testid^="${ORG_EDITOR}-row-"][data-testid$="-name"]`)
    const values = await inputs.evaluateAll(nodes =>
        nodes.map(node => (node as HTMLInputElement).value.toLowerCase())
    )
    return values
}

async function ensureUniversalLabels(page: Page) {
    await openSettingsLink(page, 'settings-org-mail-classification')
    const save = page.getByTestId(`${ORG_EDITOR}-save`)
    // Enabled once the stored taxonomy has loaded.
    await expect(save).toBeEnabled()
    const names = await orgLabelNames(page)
    if (NEEDED_LABELS.every(name => names.includes(name))) return

    await page.getByTestId(`${ORG_EDITOR}-preset`).click()
    await page.getByTestId(`${ORG_EDITOR}-preset-universal`).click()
    await save.click()
    // Disabled while the labels and rows are written, enabled again once the
    // last one is stored.
    await expect(save).toBeDisabled()
    await expect(save).toBeEnabled()
}

async function setMode(page: Page, mode: Mode) {
    await openSettingsLink(page, 'settings-account-mail-classification')
    const trigger = page.getByTestId('mail-classifier-mode')
    const shown = `Classify incoming mail: ${MODE_LABELS[mode]}`
    if ((await trigger.getAttribute('aria-label')) === shown) return

    await trigger.click()
    const written = page.waitForResponse(
        res =>
            USER_PREFERENCE_WRITE.test(res.url()) &&
            res.request().method() !== 'GET' &&
            (res.request().postData() ?? '').includes(`"${mode}"`)
    )
    await page.getByTestId(`mail-classifier-mode-${mode}`).click()
    await written
    await expect(trigger).toHaveAttribute('aria-label', shown)
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

async function openMailRules(page: Page) {
    await navigateToPackage(page, 'mail', { waitFor: page.getByTestId('package-sidebar-mounted') })
    await page.getByTestId('mail-sidebar-rules').click()
    // A rules screen visited earlier stays mounted underneath, so two headings exist.
    await expect(page.getByText('Mail rules', { exact: true }).last()).toBeVisible()
}

function ruleRow(page: Page, name: string): Locator {
    return page.locator('[data-testid="rule-row"]:visible').filter({ hasText: name })
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
    await openMailRules(page)
    await page.getByText('New rule', { exact: true }).first().click()
    await page.getByPlaceholder('Rule name').fill(opts.name)
    await selectFromMenu(page, page.getByText('Select a trigger…', { exact: true }), opts.trigger)

    await page.getByText('Field…', { exact: true }).click()
    await page.getByTestId(opts.fieldTestID).click()
    if (opts.value) await page.getByLabel('Condition value').fill(opts.value)

    await page.getByText('add action', { exact: true }).click()
    await page.getByTestId('action-option-mail:star-message').click()
    // The list shows the rule optimistically; mail delivered before the server
    // stores it would not meet the rule.
    const stored = page.waitForResponse(
        res =>
            res.url().includes('/api/collections/rules/records') &&
            res.request().method() === 'POST' &&
            res.ok()
    )
    await page.getByTestId('rule-builder-save').click()
    await stored
    await expect(ruleRow(page, opts.name)).toBeVisible()
}

// Reruns share one user, so each rule the spec builds is removed again.
async function deleteRule(page: Page, name: string) {
    await openMailRules(page)
    await ruleRow(page, name).getByTestId('rule-row-menu').click()
    await page.getByTestId('rule-row-delete').click()
    await page.getByRole('dialog').getByText('Delete', { exact: true }).last().click()
    await expect(ruleRow(page, name)).toHaveCount(0)
}

async function saveSentMessageTo(email: string) {
    await withImapClient(async client => {
        const inbox = findPersonalInbox(await listMailboxes(client))
        await appendMessage(client, inbox.replace('INBOX', 'Sent'), {
            from: TEST_USER_MAILBOX,
            to: email,
            subject: uniqueSubject('Sent to a known address'),
            body: 'Saved to Sent by a mail client.',
        })
    })
}

async function expectStarred(page: Page, subject: string) {
    await expectRowVisible(page, subject)
    await expect(
        emailRow(page, subject).getByRole('button', { name: 'Remove star', exact: true })
    ).toBeVisible()
}

test.describe('AI classification', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await enableClassifier(page)
        await ensureUniversalLabels(page)
    })

    test.afterEach(async ({ page }) => {
        await setMode(page, 'off')
    })

    test('labels a new message and shows the AI chip', async ({ page, request }) => {
        await setMode(page, 'labels')
        const { subject } = await deliverInbound(request, {
            subject: uniqueSubject('[jev:newsletters] Weekly'),
        })
        await goToInbox(page)
        await openThread(page, subject)
        await expect(page.getByTestId('mail-ai-chip')).toContainText('newsletters')
    })

    test('archives a new junk thread but not a reply', async ({ page, request }) => {
        await setMode(page, 'labels_archive')
        const fresh = await deliverInbound(request, { subject: uniqueSubject('[jev:junk] Prize') })
        // The base message has no marker, so it is never labelled; the reply
        // below joins its thread by In-Reply-To, so the thread already has a
        // message. It arrives after the junk one, so once it is listed the Inbox
        // has loaded past the archive and the junk row's absence cannot be early.
        const base = await deliverInbound(request, { subject: uniqueSubject('Thread start') })
        await expect(recentRow(page, fresh.subject)).toContainText('Labelled and archived')
        await goToInbox(page)
        await expectRowVisible(page, base.subject)
        await expect(emailRow(page, fresh.subject)).toHaveCount(0)
        await clickSidebarItem(page, 'Archive')
        await expectRowVisible(page, fresh.subject)
        await clickSidebarItem(page, 'Inbox')
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
        const name = `AI opportunity rule ${Date.now()}`
        await buildStarRule(page, {
            name,
            trigger: 'AI classifies a message',
            fieldTestID: 'condition-field-option-label_name',
            value: 'opportunities',
        })
        try {
            const { subject } = await deliverInbound(request, {
                subject: uniqueSubject('[jev:opportunities] Partnership'),
            })
            await navigateToPersonalInbox(page)
            await expectStarred(page, subject)
        } finally {
            await deleteRule(page, name)
        }
    })

    test('a has-sent-to rule on message-received runs', async ({ page, request }) => {
        await saveSentMessageTo('known@example.com')
        const name = `Known sender rule ${Date.now()}`
        await buildStarRule(page, {
            name,
            trigger: 'A message arrives',
            fieldTestID: 'condition-field-option-fields.has_sent_to_sender',
        })
        try {
            const { subject } = await deliverInbound(request, {
                subject: uniqueSubject('From a known sender'),
                fromEmail: 'known@example.com',
            })
            await navigateToPersonalInbox(page)
            await expectStarred(page, subject)
        } finally {
            await deleteRule(page, name)
        }
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
