import { expect, test } from '@playwright/test'
import { login, navigateToPackage } from '@tinycld/core/e2e-helpers'
import { deliverInbound, emailRow, expectRowVisible, uniqueSubject } from './helpers'

test.describe('Mail — Inbox', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
    })

    test('inbox shows unread badge in sidebar', async ({ page }) => {
        // Sidebar "Inbox" item shows the unread count next to its label.
        // Anchor on the Inbox sidebar entry rather than picking the bare
        // "2" text — search results, attachment counts, and other inboxes
        // all surface their own number badges that would otherwise match.
        const inboxItem = page.getByText('Inbox', { exact: true }).locator('xpath=..')
        await expect(inboxItem.getByText(/^\d+$/).first()).toBeVisible()
    })

    test('search filters threads', async ({ page, request }) => {
        const matchSubject = uniqueSubject('SearchHit')
        const otherSubject = uniqueSubject('SearchMiss')
        await deliverInbound(request, { subject: matchSubject })
        await deliverInbound(request, { subject: otherSubject })

        await page.reload()

        const searchInput = page.getByPlaceholder(/search/i)
        await expect(searchInput).toBeVisible()
        await searchInput.fill(matchSubject)

        await expectRowVisible(page, matchSubject)
        await expect(emailRow(page, otherSubject)).toHaveCount(0, { timeout: 5_000 })
    })

    // iPhone-ish viewport — the advanced-search overflow Stefan reported is
    // mobile-specific, so these tests exercise the narrow layout, not desktop.
    const MOBILE_VIEWPORT = { width: 390, height: 844 } as const

    test("advanced search shows Body and omits the removed Doesn't have / Size fields", async ({
        page,
    }) => {
        await page.setViewportSize(MOBILE_VIEWPORT)
        await page.getByTestId('advanced-search-toggle').click()

        // The "Has the words" field was renamed to "Body".
        await expect(page.getByText('Body', { exact: true })).toBeVisible()

        // The "Doesn't have" and "Size" fields were permanently removed.
        await expect(page.getByText('Has the words', { exact: true })).toHaveCount(0)
        await expect(page.getByText("Doesn't have", { exact: true })).toHaveCount(0)
        await expect(page.getByText('Size', { exact: true })).toHaveCount(0)

        // The surviving fields are still present.
        await expect(page.getByText('From', { exact: true })).toBeVisible()
        await expect(page.getByText('Subject', { exact: true })).toBeVisible()
        await expect(page.getByText('Date within', { exact: true })).toBeVisible()
    })

    test('advanced search dropdown stays within the viewport on mobile', async ({ page }) => {
        await page.setViewportSize(MOBILE_VIEWPORT)
        await page.getByTestId('advanced-search-toggle').click()

        // The dropdown panel must not overflow the right edge of the screen.
        const submit = page.getByTestId('advanced-search-submit')
        await expect(submit).toBeVisible()
        const box = await page
            .locator('[data-testid="advanced-search-input-from"]')
            .first()
            .boundingBox()
        if (!box) throw new Error('Body/From input not found')
        expect(box.x).toBeGreaterThanOrEqual(0)
        expect(box.x + box.width).toBeLessThanOrEqual(MOBILE_VIEWPORT.width)

        // The submit button (footer, far edge of the panel) must also be fully
        // on-screen — a horizontally overflowing panel would push it off.
        const submitBox = await submit.boundingBox()
        if (!submitBox) throw new Error('submit button not found')
        expect(submitBox.x + submitBox.width).toBeLessThanOrEqual(MOBILE_VIEWPORT.width)
    })

    test('advanced Body field filters by message body text', async ({ page, request }) => {
        // Two threads with distinct, unique body terms but no shared subject
        // hint — only the body distinguishes them, so this exercises the Body
        // (has_words → body_text FTS) path specifically.
        const subjectA = uniqueSubject('BodySearchA')
        const subjectB = uniqueSubject('BodySearchB')
        const uniqueBodyTerm = `zeppelin${Date.now().toString(36)}`
        await deliverInbound(request, {
            subject: subjectA,
            body: `Please review ${uniqueBodyTerm}`,
        })
        await deliverInbound(request, { subject: subjectB, body: 'Unrelated content here' })

        await page.reload()

        // Default (desktop) viewport — this test validates the body-search FTS
        // path, not the mobile overflow layout, and reuses the desktop-shaped
        // email-row assertions.
        await page.getByTestId('advanced-search-toggle').click()
        await page.getByTestId('advanced-search-input-hasWords').fill(uniqueBodyTerm)

        // Submit the advanced search.
        await page.getByTestId('advanced-search-submit').click()

        await expectRowVisible(page, subjectA)
        await expect(emailRow(page, subjectB)).toHaveCount(0, { timeout: 5_000 })
    })
})
