import { expect, type Locator, type Page, test } from '@playwright/test'
import { createInvitedUser, login, loginAs, navigateToPackage } from '@tinycld/core/e2e-helpers'
import { deliverInbound, expectRowVisible, uniqueSubject } from './helpers'

// Mail's own Rules screen: the same RulesPanel component Settings → Rules
// mounts, just scoped to scope="personal" pkgFilter="mail" (see
// tinycld/mail/screens/rules.tsx). Reached via the sidebar item rather than
// page.goto() — this file's own house rule (see CONTRIBUTING.md) — so the
// SPA never tears down mid-navigation.
async function navigateToMailRules(page: Page) {
    await navigateToPackage(page, 'mail', { waitFor: page.getByTestId('package-sidebar-mounted') })
    await page.getByTestId('mail-sidebar-rules').click()
    await expect(page.getByText('Mail rules', { exact: true })).toBeVisible()
}

// The builder's trigger/action/field/operator pickers are the house Menu
// component (a plain Pressable trigger + text-labeled items, not a native
// <select>) — click the trigger, then click the option by its exact label.
// Mirrors tinycld/tests/e2e/rules.spec.ts's selectFromMenu.
async function selectFromMenu(page: Page, trigger: Locator, optionLabel: string) {
    await trigger.click()
    await page.getByText(optionLabel, { exact: true }).click()
}

// RuleRow's DOM nesting isn't a stable number of parents from the name Text
// up to the row — find the nearest ancestor that also contains the row's
// "More actions" trigger, regardless of exactly how deep the name sits.
// Mirrors tinycld/tests/e2e/rules.spec.ts's ruleRow.
function ruleRow(page: Page, ruleName: string): Locator {
    return page
        .locator('div')
        .filter({ has: page.getByText(ruleName, { exact: true }) })
        .filter({ has: page.getByLabel('More actions') })
        .last()
}

async function openOverflowMenu(page: Page, ruleName: string) {
    await ruleRow(page, ruleName).getByLabel('More actions').click()
}

// Builds a mail rule end to end in the currently-open builder: names it,
// picks the mail "A message arrives" trigger, adds a "Subject contains
// <token>" condition, and adds a "Send me a notification" action carrying
// the token in its title (so the flagship flow has a single, unambiguous,
// user-visible signal that the rule actually ran — see the module doc at the
// bottom of this file for why the action isn't core:apply-label).
async function buildMessageArrivesRule(
    page: Page,
    opts: { ruleName: string; token: string; notifyTitle: string }
) {
    await page.getByText('New rule', { exact: true }).first().click()
    await expect(page.getByText('New rule', { exact: true }).last()).toBeVisible()

    await page.getByPlaceholder('Rule name').fill(opts.ruleName)

    await selectFromMenu(
        page,
        page.getByText('Select a trigger…', { exact: true }),
        'A message arrives'
    )

    // Choosing a record trigger offers a ready condition row, so the field
    // picker is already there — clicking "add OR group" then "add condition"
    // would build a SECOND row and make this locator ambiguous.
    //
    // The field menu defaults to no selection; picking "Subject" also seeds
    // the operator to its first legal op, which for a text field is
    // "contains" — no separate operator click needed.
    await selectFromMenu(page, page.getByText('Field…', { exact: true }), 'Subject')
    await page.getByRole('textbox').last().fill(opts.token)

    await page.getByText('add action', { exact: true }).click()
    await page.getByText('Send me a notification', { exact: true }).click()
    await page.getByText('Title').locator('..').getByRole('textbox').first().fill(opts.notifyTitle)

    await page.getByText('Save', { exact: true }).click()
    await expect(page.getByText(opts.ruleName, { exact: true })).toBeVisible()
}

test.describe('Mail — Rules screen', () => {
    test('flagship: build a mail rule, deliver a matching message, see it run', async ({
        page,
        request,
    }) => {
        await login(page)
        await navigateToMailRules(page)

        const token = uniqueSubject('RulesFlagship')
        const ruleName = `Mail flagship rule ${Date.now()}`
        const notifyTitle = `Flagship notify ${Date.now()}`

        await buildMessageArrivesRule(page, { ruleName, token, notifyTitle })

        const { subject } = await deliverInbound(request, { subject: token })

        // The engine dispatches asynchronously off the inbound webhook
        // response — poll run history rather than sleeping a fixed amount.
        await openOverflowMenu(page, ruleName)
        await page.getByText('Run history', { exact: true }).click()
        await expect(page.getByText('Matched', { exact: true }).first()).toBeVisible({
            timeout: 20_000,
        })
        await page.getByLabel('Close').click()

        // The action actually firing is the real proof the rule ran end to
        // end (not just "matched" in the abstract) — the notification bell
        // carries the unique title from this rule's action.
        const bell = page.getByLabel(/Notifications/)
        await bell.click()
        await expect(page.getByText(notifyTitle, { exact: true })).toBeVisible({ timeout: 15_000 })
        await bell.click()

        // The matching message itself did arrive and is visible in mail —
        // ties the automation run back to the concrete inbound message. We're
        // still on /mail/rules at this point, so first navigate to the inbox
        // (the rules screen has no email-row of its own) — expectRowVisible
        // (mail/tests/helpers.ts) then scopes to the visible FlashList row
        // rather than a bare getByText, since FrozenSlideStack can otherwise
        // resolve a hidden sibling screen's off-screen text node.
        await page.getByText('Inbox', { exact: true }).first().click()
        await expectRowVisible(page, subject)
    })

    test("a non-matching message logs a Didn't match run", async ({ page, request }) => {
        await login(page)
        await navigateToMailRules(page)

        const token = uniqueSubject('RulesNoMatch')
        const ruleName = `Mail no-match rule ${Date.now()}`
        const notifyTitle = `No-match notify ${Date.now()}`

        await buildMessageArrivesRule(page, { ruleName, token, notifyTitle })

        // Deliver a message that shares nothing with the rule's condition —
        // the trigger fires (mail:message-received always fires on arrival)
        // but the "Subject contains <token>" condition filters it out.
        await deliverInbound(request, { subject: uniqueSubject('Unrelated') })

        await openOverflowMenu(page, ruleName)
        await page.getByText('Run history', { exact: true }).click()
        await expect(page.getByText("Didn't match", { exact: true }).first()).toBeVisible({
            timeout: 20_000,
        })
    })

    test('dry run is gracefully unavailable to a non-admin member', async ({ page }) => {
        // TEST_USER is seeded as role "owner" (an admin) — dry-running
        // mail:message-received always succeeds for an admin (mail_messages
        // has no static owner column, so ownerFilterFor falls back to
        // "admin only, unfiltered"; see endpoints.go). Proving the graceful
        // non-admin path requires an actual non-admin, which createInvitedUser
        // provisions (default invite role is "member").
        const { user, inviteePage, close } = await createInvitedUser(page, 'mailrulesmember')
        try {
            await loginAs(inviteePage, user.email, user.password)
            await navigateToMailRules(inviteePage)

            const ruleName = `Mail dry-run rule ${Date.now()}`
            await inviteePage.getByText('New rule', { exact: true }).first().click()
            await expect(inviteePage.getByText('New rule', { exact: true }).last()).toBeVisible()
            await inviteePage.getByPlaceholder('Rule name').fill(ruleName)
            await selectFromMenu(
                inviteePage,
                inviteePage.getByText('Select a trigger…', { exact: true }),
                'A message arrives'
            )

            await inviteePage.getByText('Test against recent items', { exact: true }).click()

            // DryRunPanel's ErrorNotice: the server's "cannot be scoped"
            // message plus the muted follow-up line — never rendered as an
            // error toast (core/components/rules/DryRunPanel.tsx).
            await expect(
                inviteePage.getByText('Org admins can test this trigger.', { exact: true })
            ).toBeVisible({ timeout: 15_000 })
        } finally {
            await close()
        }
    })

    test('the trigger menu only offers mail (plus core built-ins), never another package', async ({
        page,
    }) => {
        // Task 4's preset filtering: mail's "New rule" seeds presetPkg="mail"
        // (RulesPanel's pkgFilter flows into openCreate → builder.presetPkg),
        // so TriggerCard's menu (grouped by Menu.Label{pkg}) must show only
        // the "mail" group plus core's scope-neutral synthetic triggers
        // (manual/schedule) — never another feature package's triggers, even
        // if one happens to be installed in this workspace assembly.
        await login(page)
        await navigateToMailRules(page)

        await page.getByText('New rule', { exact: true }).first().click()
        await expect(page.getByText('New rule', { exact: true }).last()).toBeVisible()

        await page.getByText('Select a trigger…', { exact: true }).click()

        // The trigger this preset must expose.
        await expect(page.getByText('A message arrives', { exact: true })).toBeVisible()

        // Menu.Label renders the raw pkg slug (e.g. "mail", "core") — assert
        // the mail group is present and no other feature package's group
        // leaked in. Robust to a lean assembly: this only checks slugs that
        // would exist as automation-owning packages in the ecosystem, it
        // doesn't require any of them to actually be installed.
        await expect(page.getByText('mail', { exact: true })).toBeVisible()
        const otherPackageSlugs = ['calendar', 'drive', 'contacts', 'calc', 'text', 'cards']
        for (const slug of otherPackageSlugs) {
            await expect(page.getByText(slug, { exact: true })).not.toBeVisible()
        }

        // Pick the trigger to close the popover (its own backdrop otherwise
        // intercepts pointer events for anything behind it, including the
        // dialog's Cancel button) before dismissing the builder.
        await page.getByText('A message arrives', { exact: true }).click()
        await page.getByText('Cancel', { exact: true }).click()
    })

    test('a mail-created rule also appears in Settings → Rules (My rules)', async ({ page }) => {
        await login(page)
        await navigateToMailRules(page)

        const token = uniqueSubject('RulesParity')
        const ruleName = `Mail parity rule ${Date.now()}`
        const notifyTitle = `Parity notify ${Date.now()}`

        await buildMessageArrivesRule(page, { ruleName, token, notifyTitle })

        await page.getByTestId('nav-settings').click()
        await page.getByText('Rules', { exact: true }).first().click()
        // "My rules" is the default segment on load (app/(app)/settings/rules.tsx) —
        // no tab click needed.
        await expect(page.getByText('My rules', { exact: true })).toBeVisible()

        // FrozenSlideStack keeps the mail Rules screen (/mail/rules) mounted
        // underneath Settings → Rules, and both panels include this rule
        // (mail's pkgFilter="mail" view and settings' unfiltered personal
        // view) — React Native Web stacks the inactive screen rather than
        // `display:none`-ing it, so a plain `:visible` filter doesn't
        // disambiguate either. Scope to the settings screen specifically: its
        // "My rules"/"Organization" segment pills are unique to this screen
        // (mail's Rules screen has no segment picker), so the nearest
        // ancestor containing both the rule name and "My rules" is the
        // settings panel's copy.
        const settingsRulesPanel = page
            .locator('div')
            .filter({ has: page.getByText('My rules', { exact: true }) })
            .filter({ has: page.getByText(ruleName, { exact: true }) })
            .last()
        await expect(settingsRulesPanel.getByText(ruleName, { exact: true })).toBeVisible()
        await expect(
            settingsRulesPanel.getByText('A message arrives', { exact: false }).first()
        ).toBeVisible()
    })
})

// A note on why the flagship flow's action is "Send me a notification"
// rather than core's "Apply label" (the plan's suggested example): mail's
// label UI (sidebar filter, list-row chips, thread header) reads
// label_assignments scoped to collection "mail_thread_state" exclusively
// (mail/tinycld/mail/hooks/useLabels.ts, useThreadListItems.ts,
// useSearchThreadItems.ts). core:apply-label's record-op writes
// collection/record_id from the trigger record's own identity
// (core/lib/automation/core-defs.ts + core/server/automation/actions.go's
// resolveSetValue "record-id"/"collection" contexts) — for
// mail:message-received that's a mail_messages row, never mail_thread_state.
// Verified empirically: building this exact rule and delivering a matching
// message creates a label_assignments row with collection "mail_messages",
// which the mail UI never reads, so no user-visible label appears anywhere.
// Making apply-label resolve through a trigger's related "display" record
// would need a new engine primitive (mirroring OwnerResolver, but for the
// record-op target) — out of scope for this e2e/help task. Filed for
// follow-up; the notify action instead exercises the identical build →
// trigger → condition → action → run-history path with an assertion that is
// actually true today.
