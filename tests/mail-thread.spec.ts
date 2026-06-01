import { expect, test } from '@playwright/test'
import { clickSidebarItem, login, navigateToPackage } from '../../app/tests/e2e/helpers'
import {
    deliverInbound,
    expectRowVisible,
    navigateToPersonalInbox,
    openThread,
    threadDetail,
    uniqueSubject,
} from './helpers'

test.describe('Mail — Thread Detail', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
        // The seed creates a shared "Support" mailbox in addition to the
        // personal one, so the default landing page is "All Inboxes". These
        // tests reason about a thread's *folder* (inbox vs. archive vs.
        // trash), so they need a scoped view first.
        await navigateToPersonalInbox(page)
    })

    test('archive thread from detail', async ({ page, request }) => {
        const subject = uniqueSubject('Archive')
        await deliverInbound(request, { subject })
        await page.reload()
        await navigateToPersonalInbox(page)

        await openThread(page, subject)
        await threadDetail(page).getByLabel('Archive').click()
        await page.waitForURL(url => !url.pathname.includes('/mail/'), { timeout: 10_000 })

        await clickSidebarItem(page, 'Archive')
        await expectRowVisible(page, subject)
    })

    test('delete thread from detail', async ({ page, request }) => {
        const subject = uniqueSubject('Delete')
        await deliverInbound(request, { subject })
        await page.reload()
        await navigateToPersonalInbox(page)

        await openThread(page, subject)
        await threadDetail(page).getByLabel('Delete').click()
        await page.waitForURL(url => !url.pathname.includes('/mail/'), { timeout: 10_000 })

        await clickSidebarItem(page, 'Trash')
        await expectRowVisible(page, subject)
    })

    // Reply / reply-all pre-fill the To field, so focus belongs in the body
    // editor — the user wants to type their message, not edit recipients.
    // (Forward starts with an empty To and keeps focus on the recipient field.)
    test('reply focuses the message body, not the To field', async ({ page, request }) => {
        const subject = uniqueSubject('ReplyFocus')
        await deliverInbound(request, { subject })
        await page.reload()
        await navigateToPersonalInbox(page)

        await openThread(page, subject)
        await threadDetail(page).getByText('Reply', { exact: true }).click()

        // The inline reply form is a lazy chunk; wait for the editor to paint.
        const editor = page.locator('.tinycld-mail-editor .ProseMirror')
        await expect(editor).toBeVisible({ timeout: 15_000 })

        // The body editor — not a recipient <input> — owns focus on open.
        await expect(editor).toBeFocused()
    })
})
