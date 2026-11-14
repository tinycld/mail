import { type APIRequestContext, expect, type Locator, type Page, test } from '@playwright/test'
import { clickSidebarItem, login, navigateToPackage } from '@tinycld/core/e2e-helpers'
import {
    deliverInbound,
    emailRow,
    expectRowVisible,
    navigateToPersonalInbox,
    openThread,
    uniqueSubject,
} from './helpers'

// Every button in the mail screens, pressed for real, with the result it must
// have. Each control is found by its accessible role and name, so a control
// that a screen reader cannot name fails here too.
//
// A menu is checked for more than "the item is in the DOM": Playwright counts
// an opacity-0 element as visible and clicks it, so a menu that opens
// invisibly would pass a plain click. `chooseMenuItem` asserts the row is
// painted and on screen before it clicks.
//
// Not here, because other specs already press them: sidebar folder links
// (mail-navigation), sidebar label filter (mail-labels), list pagination
// (mail-paging), advanced search (mail-inbox), attachment download
// (mail-attachments), the Rules screen's own buttons (mail-rules-screen),
// and the settings screens (their own specs).

const PHONE = { width: 390, height: 844 }
const SENDER_NAME = 'Test Sender'
const SENDER_EMAIL = 'sender@example.com'

function visibleDetail(page: Page): Locator {
    return page.getByTestId('mail-thread-detail').filter({ visible: true })
}

async function fittedToolbar(scope: Locator): Promise<Locator> {
    const toolbar = scope.getByTestId('toolbar').filter({ visible: true }).first()
    const row = toolbar.getByTestId('toolbar-row')
    await expect(row).toHaveCSS('opacity', '1')
    // The row measures its width on layout. After a viewport shrinks, it shows
    // the fit for the old width for a frame or two. A button that fit inline
    // before then folds into More actions. The fit for the current width is in
    // place when every slot ends inside the row. The 1px slack is the same
    // FIT_TOLERANCE that fitToolbarItems allows for layout rounding.
    await expect
        .poll(() =>
            row.evaluate(el => {
                const right = el.getBoundingClientRect().right + 1
                return Array.from(el.children).every(c => c.getBoundingClientRect().right <= right)
            })
        )
        .toBe(true)
    return toolbar
}

function threadToolbar(page: Page): Promise<Locator> {
    return fittedToolbar(visibleDetail(page))
}

// The header row of the one message in a fresh thread: the nearest element
// that holds both the sender's name and the message's More options button.
function messageHeader(page: Page): Locator {
    return visibleDetail(page)
        .locator('div')
        .filter({ has: page.getByText(SENDER_NAME, { exact: true }) })
        .filter({ has: page.getByRole('button', { name: 'More options', exact: true }) })
        .last()
}

function composeWindow(page: Page): Locator {
    return page
        .locator('div')
        .filter({ has: page.getByRole('button', { name: 'Close compose', exact: true }) })
        .filter({ has: page.getByTestId('mail-compose-to') })
        .last()
}

function editor(page: Page): Locator {
    return page.locator('.tinycld-mail-editor .ProseMirror').filter({ visible: true }).first()
}

async function expectOnScreen(locator: Locator) {
    await expect(locator).toBeVisible()
    await expect
        .poll(() =>
            locator.evaluate(el => {
                let opacity = 1
                for (let node: Element | null = el; node; node = node.parentElement) {
                    opacity *= Number(getComputedStyle(node).opacity)
                }
                return opacity
            })
        )
        .toBeGreaterThan(0.99)
    await expect(locator).toBeInViewport()
}

async function chooseMenuItem(page: Page, name: string) {
    const item = page.getByRole('menuitem', { name, exact: true }).filter({ visible: true }).last()
    await expectOnScreen(item)
    await item.click()
}

// A toolbar folds what does not fit into its More actions menu, so a button
// is pressed inline when it is shown and through that menu when it is not.
async function pressToolbarAction(page: Page, toolbar: Locator, name: string) {
    const inline = toolbar.getByRole('button', { name, exact: true })
    if (await inline.isVisible()) {
        await inline.click()
        return
    }
    await toolbar.getByRole('button', { name: 'More actions', exact: true }).click()
    await chooseMenuItem(page, name)
}

async function openFreshThread(
    page: Page,
    request: APIRequestContext,
    label: string,
    date?: Date
): Promise<string> {
    const subject = uniqueSubject(label)
    await deliverInbound(request, { subject, date })
    await page.reload()
    await navigateToPersonalInbox(page)
    await openThread(page, subject)
    await expect(visibleDetail(page).getByText(subject).first()).toBeVisible()
    return subject
}

async function deliverToInbox(page: Page, request: APIRequestContext, label: string) {
    const subject = uniqueSubject(label)
    await deliverInbound(request, { subject })
    await page.reload()
    await navigateToPersonalInbox(page)
    await expectRowVisible(page, subject)
    return subject
}

// A fresh thread reported as spam, with the Spam folder open.
async function deliverToSpam(page: Page, request: APIRequestContext, label: string) {
    const subject = await deliverToInbox(page, request, label)
    await selectRow(page, subject)
    await pressToolbarAction(page, await listToolbar(page), 'Report spam')
    await expect(emailRow(page, subject)).toHaveCount(0)
    await expectInFolder(page, 'Spam', subject)
    return subject
}

async function expectInFolder(page: Page, folder: string, subject: string) {
    await clickSidebarItem(page, folder)
    await expectRowVisible(page, subject)
}

async function expectListShown(page: Page) {
    await expect(page.locator('[data-testid="email-row"]:visible').first()).toBeVisible()
}

// The first label in the sidebar's Labels section, so the spec does not
// depend on which labels the seed created.
async function firstLabelName(page: Page): Promise<string> {
    const item = page
        .getByTestId(/^mail-sidebar-label-/)
        .filter({ visible: true })
        .first()
    await expect(item).toBeVisible()
    return (await item.innerText()).trim()
}

async function selectRow(page: Page, subject: string) {
    const row = emailRow(page, subject)
    await row.getByRole('checkbox').click()
    await expect(page.getByText('1 selected', { exact: true })).toBeVisible()
}

async function listToolbar(page: Page): Promise<Locator> {
    return fittedToolbar(page.locator('body'))
}

// The Reply / Reply all / Forward buttons under the last message.
function replyBarButton(page: Page, name: string): Locator {
    return visibleDetail(page).getByText(name, { exact: true }).last()
}

// The phone list renders its own row, without the desktop row's testID.
function phoneRow(page: Page, subject: string): Locator {
    return page
        .locator('[data-testid^="mail-thread-row-"]:visible')
        .filter({ hasText: subject })
        .first()
}

async function hoverAction(page: Page, subject: string, name: string): Promise<Locator> {
    const row = emailRow(page, subject)
    await row.hover()
    const action = row.getByLabel(name, { exact: true })
    await expect(action).toBeVisible()
    return action
}

async function openCompose(page: Page) {
    await page.getByText('Compose', { exact: true }).first().click()
    await expect(page.getByTestId('mail-compose-to')).toBeVisible()
}

async function startNewMessage(page: Page) {
    await openCompose(page)
    await editor(page).click()
}

test.describe('Mail buttons — thread toolbar', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
        await navigateToPersonalInbox(page)
    })

    test('Back returns to the list', async ({ page, request }) => {
        await openFreshThread(page, request, 'BtnBack')
        await pressToolbarAction(page, await threadToolbar(page), 'Back')
        await expectListShown(page)
        await expect(visibleDetail(page)).toHaveCount(0)
    })

    test('Archive files the thread under Archive', async ({ page, request }) => {
        const subject = await openFreshThread(page, request, 'BtnArchive')
        await pressToolbarAction(page, await threadToolbar(page), 'Archive')
        await expectListShown(page)
        await expect(emailRow(page, subject)).toHaveCount(0)
        await expectInFolder(page, 'Archive', subject)
    })

    test('Report spam files the thread under Spam', async ({ page, request }) => {
        const subject = await openFreshThread(page, request, 'BtnSpam')
        await pressToolbarAction(page, await threadToolbar(page), 'Report spam')
        await expectListShown(page)
        await expect(emailRow(page, subject)).toHaveCount(0)
        await expectInFolder(page, 'Spam', subject)
    })

    test('Not spam returns a spam thread to Inbox', async ({ page, request }) => {
        const subject = await deliverToSpam(page, request, 'BtnNotSpam')
        await openThread(page, subject)
        const toolbar = await threadToolbar(page)
        await expect(toolbar.getByRole('button', { name: 'Report spam', exact: true })).toHaveCount(
            0
        )
        await pressToolbarAction(page, toolbar, 'Not spam')
        await expectListShown(page)
        await expect(emailRow(page, subject)).toHaveCount(0)
        await expectInFolder(page, 'Inbox', subject)
    })

    test('Delete files the thread under Trash', async ({ page, request }) => {
        const subject = await openFreshThread(page, request, 'BtnDelete')
        await pressToolbarAction(page, await threadToolbar(page), 'Delete')
        await expectListShown(page)
        await expect(emailRow(page, subject)).toHaveCount(0)
        await expectInFolder(page, 'Trash', subject)
    })

    test('Move to opens a menu whose folder moves the thread', async ({ page, request }) => {
        const subject = await openFreshThread(page, request, 'BtnMoveTo')
        await pressToolbarAction(page, await threadToolbar(page), 'Move to')
        await chooseMenuItem(page, 'Archive')
        await expectListShown(page)
        await expect(emailRow(page, subject)).toHaveCount(0)
        await expectInFolder(page, 'Archive', subject)
    })

    test('Labels opens a menu that adds and removes a label', async ({ page, request }) => {
        const label = await firstLabelName(page)
        await openFreshThread(page, request, 'BtnLabels')
        const detail = visibleDetail(page)
        await expect(detail.getByText(label, { exact: true })).toHaveCount(0)

        await pressToolbarAction(page, await threadToolbar(page), 'Labels')
        await chooseMenuItem(page, label)
        await expect(detail.getByText(label, { exact: true })).toBeVisible()

        await pressToolbarAction(page, await threadToolbar(page), 'Labels')
        await chooseMenuItem(page, label)
        await expect(detail.getByText(label, { exact: true })).toHaveCount(0)
    })

    test('Mark as unread marks the thread unread', async ({ page, request }) => {
        const subject = await openFreshThread(page, request, 'BtnUnread')
        const toolbar = await threadToolbar(page)
        await pressToolbarAction(page, toolbar, 'Mark as unread')
        await expect(
            toolbar.getByRole('button', { name: 'Mark as read', exact: true })
        ).toBeVisible()

        await pressToolbarAction(page, toolbar, 'Back')
        await expectListShown(page)
        await expect(await hoverAction(page, subject, 'Mark as read')).toBeVisible()
    })

    test('Add star stars the thread', async ({ page, request }) => {
        const subject = await openFreshThread(page, request, 'BtnStar')
        const toolbar = await threadToolbar(page)
        await pressToolbarAction(page, toolbar, 'Add star')
        await expect(
            toolbar.getByRole('button', { name: 'Remove star', exact: true })
        ).toBeVisible()
        await expectInFolder(page, 'Starred', subject)
    })

    test('Remove star unstars the thread', async ({ page, request }) => {
        const subject = await openFreshThread(page, request, 'BtnUnstar')
        const toolbar = await threadToolbar(page)
        await pressToolbarAction(page, toolbar, 'Add star')
        await pressToolbarAction(page, toolbar, 'Remove star')
        await expect(toolbar.getByRole('button', { name: 'Add star', exact: true })).toBeVisible()
        await clickSidebarItem(page, 'Starred')
        await expect(emailRow(page, subject)).toHaveCount(0)
    })

    // Not built yet: the button is a disabled stub. It must open a search for
    // mail from this sender, the way the advanced search panel does.
    test('Filter messages like these opens a search for this sender', async ({ page, request }) => {
        await openFreshThread(page, request, 'BtnFilter')
        const toolbar = await threadToolbar(page)
        await expect(
            toolbar.getByRole('button', { name: 'Filter messages like these', exact: true })
        ).toBeEnabled()
        await pressToolbarAction(page, toolbar, 'Filter messages like these')
        const panel = page.getByTestId('advanced-search-panel')
        await expect(panel).toBeVisible()
        await expect(panel.getByTestId('advanced-search-input-from')).toHaveValue(SENDER_EMAIL)
    })

    test('Forward all opens a forward of the thread', async ({ page, request }) => {
        const subject = await openFreshThread(page, request, 'BtnForwardAll')
        await pressToolbarAction(page, await threadToolbar(page), 'Forward all')
        await expect(page.getByTestId('mail-compose-subject')).toHaveValue(`Fwd: ${subject}`)
        await expect(page.getByTestId('mail-compose-to')).toHaveValue('')
    })

    // Other specs deliver to the same inbox in parallel, so the thread one step
    // older is not known in advance: the check is that Older leaves this
    // thread and Newer comes back to it.
    test('Older and Newer step between threads', async ({ page, request }) => {
        const subject = await openFreshThread(page, request, 'BtnOlderNewer')
        const threadUrl = page.url()

        await pressToolbarAction(page, await threadToolbar(page), 'Older')
        await expect(page).not.toHaveURL(threadUrl)
        await expect(visibleDetail(page).getByText(subject)).toHaveCount(0)

        await pressToolbarAction(page, await threadToolbar(page), 'Newer')
        await expect(page).toHaveURL(threadUrl)
        await expect(visibleDetail(page).getByText(subject).first()).toBeVisible()
    })
})

test.describe('Mail buttons — message header and reply bar', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
        await navigateToPersonalInbox(page)
    })

    test('the header star stars the thread', async ({ page, request }) => {
        const subject = await openFreshThread(page, request, 'BtnHeaderStar')
        const header = messageHeader(page)
        await header.getByRole('button', { name: 'Add star', exact: true }).click()
        await expect(header.getByRole('button', { name: 'Remove star', exact: true })).toBeVisible()
        await expectInFolder(page, 'Starred', subject)
    })

    test('the header Reply opens a reply', async ({ page, request }) => {
        const subject = await openFreshThread(page, request, 'BtnHeaderReply')
        await messageHeader(page).getByRole('button', { name: 'Reply', exact: true }).click()
        await expect(page.getByTestId('mail-compose-subject')).toHaveValue(`Re: ${subject}`)
    })

    test('Collapse hides the body and the snippet shows it again', async ({ page, request }) => {
        await openFreshThread(page, request, 'BtnCollapse')
        const detail = visibleDetail(page)
        const body = detail.locator('iframe[title="Email body"]')
        await expect(body).toBeVisible()

        await messageHeader(page).getByRole('button', { name: 'Collapse', exact: true }).click()
        await expect(body).toHaveCount(0)

        await messageHeader(page).getByRole('button', { name: 'Expand', exact: true }).click()
        await expect(body).toBeVisible()

        await messageHeader(page).getByRole('button', { name: 'Collapse', exact: true }).click()
        await detail.getByText('Test body', { exact: true }).click()
        await expect(body).toBeVisible()
    })

    for (const { item, subjectPrefix } of [
        { item: 'Reply', subjectPrefix: 'Re: ' },
        { item: 'Reply all', subjectPrefix: 'Re: ' },
        { item: 'Forward', subjectPrefix: 'Fwd: ' },
    ]) {
        test(`More options → ${item} opens it without collapsing the message`, async ({
            page,
            request,
        }) => {
            const subject = await openFreshThread(page, request, `BtnMore${item.replace(' ', '')}`)
            const body = visibleDetail(page).locator('iframe[title="Email body"]')
            await expect(body).toBeVisible()

            await messageHeader(page)
                .getByRole('button', { name: 'More options', exact: true })
                .click()
            await chooseMenuItem(page, item)

            await expect(page.getByTestId('mail-compose-subject')).toHaveValue(
                `${subjectPrefix}${subject}`
            )
            await expect(body).toBeVisible()
        })
    }

    for (const { button, subjectPrefix, to } of [
        { button: 'Reply', subjectPrefix: 'Re: ', to: SENDER_EMAIL },
        { button: 'Reply all', subjectPrefix: 'Re: ', to: SENDER_EMAIL },
        { button: 'Forward', subjectPrefix: 'Fwd: ', to: '' },
    ]) {
        test(`the reply bar's ${button} opens the inline form`, async ({ page, request }) => {
            const subject = await openFreshThread(page, request, `BtnBar${button.replace(' ', '')}`)
            await replyBarButton(page, button).click()
            await expect(editor(page)).toBeVisible()
            await expect(page.getByTestId('mail-compose-subject')).toHaveValue(
                `${subjectPrefix}${subject}`
            )
            if (to) await expect(visibleDetail(page).getByText(to).last()).toBeVisible()
        })
    }

    test('the inline form Discard closes it', async ({ page, request }) => {
        await openFreshThread(page, request, 'BtnInlineDiscard')
        const detail = visibleDetail(page)
        await replyBarButton(page, 'Reply all').click()
        await expect(editor(page)).toBeVisible()
        await detail
            .getByTestId('toolbar')
            .getByRole('button', { name: 'Discard', exact: true })
            .click()
        await expect(page.getByTestId('mail-compose-subject')).toHaveCount(0)
        await expect(replyBarButton(page, 'Reply all')).toBeVisible()
    })

    test('the inline form Send posts the reply', async ({ page, request }) => {
        await openFreshThread(page, request, 'BtnInlineSend')
        await replyBarButton(page, 'Reply').click()
        await editor(page).click()
        await page.keyboard.type('Reply body')

        const send = page.waitForRequest(
            req => req.url().includes('/api/mail/send') && req.method() === 'POST'
        )
        await page
            .getByRole('button', { name: 'Send', exact: true })
            .filter({ visible: true })
            .click()
        expect((await send).postDataJSON().subject).toMatch(/^Re: /)
    })
})

test.describe('Mail buttons — list', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
        await navigateToPersonalInbox(page)
    })

    test('the row checkbox selects the row', async ({ page, request }) => {
        const subject = await deliverToInbox(page, request, 'BtnRowCheck')
        await selectRow(page, subject)
        await emailRow(page, subject).getByRole('checkbox').click()
        await expect(page.getByText('1 selected', { exact: true })).toHaveCount(0)
    })

    test('Select all selects every row on the page', async ({ page, request }) => {
        await deliverToInbox(page, request, 'BtnSelectAll')
        const toolbar = await listToolbar(page)
        await toolbar.getByRole('checkbox', { name: 'Select all', exact: true }).click()
        await expect(page.getByText(/^\d+ selected$/)).toBeVisible()
    })

    test('Refresh reloads the list', async ({ page }) => {
        const toolbar = await listToolbar(page)
        const reload = page.waitForRequest(req =>
            req.url().includes('/api/collections/mail_thread_state/records')
        )
        await pressToolbarAction(page, toolbar, 'Refresh')
        await reload
        await expectListShown(page)
    })

    test('the list help button opens help', async ({ page }) => {
        const toolbar = await listToolbar(page)
        await toolbar.getByRole('button', { name: 'Open help', exact: true }).click()
        await expect(page.getByText('Reading threads', { exact: true }).first()).toBeVisible()
    })

    test('the row star stars the thread', async ({ page, request }) => {
        const subject = await deliverToInbox(page, request, 'BtnRowStar')
        await emailRow(page, subject).getByRole('button', { name: 'Add star', exact: true }).click()
        await expect(
            emailRow(page, subject).getByRole('button', { name: 'Remove star', exact: true })
        ).toBeVisible()
        await expectInFolder(page, 'Starred', subject)
    })

    test('the row Mark as read / unread toggles the thread', async ({ page, request }) => {
        const subject = await deliverToInbox(page, request, 'BtnRowRead')
        await (await hoverAction(page, subject, 'Mark as read')).click()
        await (await hoverAction(page, subject, 'Mark as unread')).click()
        await expect(await hoverAction(page, subject, 'Mark as read')).toBeVisible()
    })

    test('the row Move to Inbox restores an archived thread', async ({ page, request }) => {
        const subject = await deliverToInbox(page, request, 'BtnRowRestore')
        await (await hoverAction(page, subject, 'Archive')).click()
        await expect(emailRow(page, subject)).toHaveCount(0)
        await clickSidebarItem(page, 'Archive')
        await expectRowVisible(page, subject)
        await (await hoverAction(page, subject, 'Move to Inbox')).click()
        await expect(emailRow(page, subject)).toHaveCount(0)
        await expectInFolder(page, 'Inbox', subject)
    })

    test('the row Move to Inbox restores a deleted thread', async ({ page, request }) => {
        const subject = await deliverToInbox(page, request, 'BtnRowUntrash')
        await (await hoverAction(page, subject, 'Delete')).click()
        await expect(emailRow(page, subject)).toHaveCount(0)
        await clickSidebarItem(page, 'Trash')
        await expectRowVisible(page, subject)
        await (await hoverAction(page, subject, 'Move to Inbox')).click()
        await expect(emailRow(page, subject)).toHaveCount(0)
        await expectInFolder(page, 'Inbox', subject)
    })

    test('the row Not spam returns a spam thread to Inbox', async ({ page, request }) => {
        const subject = await deliverToSpam(page, request, 'BtnRowNotSpam')
        await (await hoverAction(page, subject, 'Not spam')).click()
        await expect(emailRow(page, subject)).toHaveCount(0)
        await expectInFolder(page, 'Inbox', subject)
    })

    test('bulk Not spam returns the selection to Inbox', async ({ page, request }) => {
        const subject = await deliverToSpam(page, request, 'BtnBulkNotSpam')
        await selectRow(page, subject)
        await pressToolbarAction(page, await listToolbar(page), 'Not spam')
        await expect(emailRow(page, subject)).toHaveCount(0)
        await expectInFolder(page, 'Inbox', subject)
    })

    for (const { action, folder } of [
        { action: 'Archive', folder: 'Archive' },
        { action: 'Report spam', folder: 'Spam' },
        { action: 'Delete', folder: 'Trash' },
    ]) {
        test(`bulk ${action} files the selection under ${folder}`, async ({ page, request }) => {
            const subject = await deliverToInbox(page, request, `BtnBulk${folder}`)
            await selectRow(page, subject)
            await pressToolbarAction(page, await listToolbar(page), action)
            await expect(emailRow(page, subject)).toHaveCount(0)
            await expectInFolder(page, folder, subject)
        })
    }

    test('bulk Mark as read and unread toggle the selection', async ({ page, request }) => {
        const subject = await deliverToInbox(page, request, 'BtnBulkRead')
        await selectRow(page, subject)
        // A bulk action clears the selection, so the row's own hover action
        // shows the new state.
        await pressToolbarAction(page, await listToolbar(page), 'Mark as read')
        await expect(page.getByText('1 selected', { exact: true })).toHaveCount(0)
        await hoverAction(page, subject, 'Mark as unread')
        await selectRow(page, subject)
        await pressToolbarAction(page, await listToolbar(page), 'Mark as unread')
        await hoverAction(page, subject, 'Mark as read')
    })

    test('bulk Move to moves the selection', async ({ page, request }) => {
        const subject = await deliverToInbox(page, request, 'BtnBulkMove')
        await selectRow(page, subject)
        await pressToolbarAction(page, await listToolbar(page), 'Move to')
        await chooseMenuItem(page, 'Archive')
        await expect(emailRow(page, subject)).toHaveCount(0)
        await expectInFolder(page, 'Archive', subject)
    })

    test('bulk Labels adds a label to the selection', async ({ page, request }) => {
        const label = await firstLabelName(page)
        const subject = await deliverToInbox(page, request, 'BtnBulkLabel')
        await selectRow(page, subject)
        await pressToolbarAction(page, await listToolbar(page), 'Labels')
        await chooseMenuItem(page, label)
        await openThread(page, subject)
        await expect(visibleDetail(page).getByText(label, { exact: true })).toBeVisible()
    })

    test('bulk Add star stars the selection', async ({ page, request }) => {
        const subject = await deliverToInbox(page, request, 'BtnBulkStar')
        await selectRow(page, subject)
        await pressToolbarAction(page, await listToolbar(page), 'Add star')
        await expectInFolder(page, 'Starred', subject)
    })
})

test.describe('Mail buttons — sidebar', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
    })

    test('All Inboxes opens the unified inbox', async ({ page }) => {
        await clickSidebarItem(page, 'All Inboxes')
        await expect(page).toHaveURL(/folder=all-inboxes/)
        await expectListShown(page)
    })

    test('the Labels settings button opens the label manager', async ({ page }) => {
        await page.getByRole('button', { name: 'Manage labels', exact: true }).click()
        const dialog = page.getByRole('dialog')
        await expect(dialog.getByText('Labels', { exact: true })).toBeVisible()
        await expect(dialog.getByText('New label...', { exact: true })).toBeVisible()
    })

    test('Rules opens the rules screen', async ({ page }) => {
        await page.getByTestId('mail-sidebar-rules').click()
        await expect(page.getByText('Mail rules', { exact: true })).toBeVisible()
    })

    test('Help opens the mail help topics', async ({ page }) => {
        await clickSidebarItem(page, 'Help')
        await expect(page.getByText('Getting started with Mail', { exact: true })).toBeVisible()
    })
})

test.describe('Mail buttons — compose window', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
    })

    test('Minimize hides the form and pressing it again restores it', async ({ page }) => {
        await openCompose(page)
        const minimize = composeWindow(page).getByRole('button', { name: 'Minimize', exact: true })
        await minimize.click()
        await expect(page.getByTestId('mail-compose-to')).toBeHidden()
        await minimize.click()
        await expect(page.getByTestId('mail-compose-to')).toBeVisible()
    })

    test('Maximize widens the window', async ({ page }) => {
        await openCompose(page)
        const before = (await composeWindow(page).boundingBox())?.width ?? 0
        await composeWindow(page).getByRole('button', { name: 'Maximize', exact: true }).click()
        await expect
            .poll(async () => (await composeWindow(page).boundingBox())?.width ?? 0)
            .toBeGreaterThan(before)
    })

    test('the compose help button opens the composing topic', async ({ page }) => {
        await openCompose(page)
        await composeWindow(page).getByRole('button', { name: 'Open help', exact: true }).click()
        await expect(page.getByText('Composing and sending email', { exact: true })).toBeVisible()
    })

    test('Cc and Bcc show their fields', async ({ page }) => {
        await openCompose(page)
        await composeWindow(page).getByRole('button', { name: 'Cc', exact: true }).click()
        await expect(page.getByTestId('mail-compose-cc')).toBeVisible()
        await composeWindow(page).getByRole('button', { name: 'Bcc', exact: true }).click()
        await expect(page.getByTestId('mail-compose-bcc')).toBeVisible()
    })

    for (const { button, selector } of [
        { button: 'Bold', selector: 'strong' },
        { button: 'Italic', selector: 'em' },
        { button: 'Underline', selector: 'u' },
        { button: 'Bullet list', selector: 'ul li' },
        { button: 'Numbered list', selector: 'ol li' },
        { button: 'Blockquote', selector: 'blockquote' },
    ]) {
        test(`${button} formats the typed text`, async ({ page }) => {
            await startNewMessage(page)
            await pressToolbarAction(page, await fittedToolbar(composeWindow(page)), button)
            await page.keyboard.type('formatted')
            await expect(editor(page).locator(selector)).toHaveText('formatted')
        })
    }

    test('Link turns the selection into a link', async ({ page }) => {
        await startNewMessage(page)
        await page.keyboard.type('linked')
        // Select All, not Shift+Home. ProseMirror applies its own Select All
        // command to the editor state at once. Shift+Home moves only the DOM
        // selection, which ProseMirror reads later on `selectionchange`. When
        // the editor gets focus, ProseMirror also sets a 20ms timer that writes
        // the state selection back to the DOM. Synthetic keys come faster than a
        // person types, so that timer can run after Shift+Home but before the
        // editor reads it. The timer then collapses the selection, and Link
        // only arms a stored mark. Nothing in the page shows when the timer has
        // run, so the test cannot wait for it.
        await page.keyboard.press('ControlOrMeta+a')
        page.once('dialog', dialog => dialog.accept('https://example.com/'))
        await pressToolbarAction(page, await fittedToolbar(composeWindow(page)), 'Link')
        await expect(editor(page).locator('a[href="https://example.com/"]')).toHaveText('linked')
    })

    test('Attach opens the file picker', async ({ page }) => {
        await openCompose(page)
        const chooser = page.waitForEvent('filechooser')
        await pressToolbarAction(page, await fittedToolbar(composeWindow(page)), 'Attach')
        await chooser
    })

    test('Discard closes the window', async ({ page }) => {
        await startNewMessage(page)
        await page.keyboard.type('Throw this away')
        await pressToolbarAction(page, await fittedToolbar(composeWindow(page)), 'Discard')
        await expect(page.getByTestId('mail-compose-to')).toHaveCount(0)
    })

    test('Send posts the message', async ({ page }) => {
        const subject = uniqueSubject('BtnSend')
        await openCompose(page)
        await page.getByTestId('mail-compose-to').fill('friend@example.net, ')
        await page.getByTestId('mail-compose-subject').fill(subject)
        await editor(page).click()
        await page.keyboard.type('Send body')

        const send = page.waitForRequest(
            req => req.url().includes('/api/mail/send') && req.method() === 'POST'
        )
        await composeWindow(page).getByRole('button', { name: 'Send', exact: true }).click()
        expect((await send).postDataJSON().subject).toBe(subject)
    })
})

// Below the mobile breakpoint a toolbar menu opens as a sheet and the list
// renders phone rows and a drawer, which is different code from desktop.
test.describe('Mail buttons — phone', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
        await navigateToPersonalInbox(page)
    })

    test('the drawer toggle opens the mail sidebar', async ({ page }) => {
        await page.setViewportSize(PHONE)
        await page.getByTestId('drawer-toggle').filter({ visible: true }).click()
        await expect(page.getByText('All Inboxes', { exact: true }).first()).toBeVisible()
    })

    test('the phone row star stars the thread', async ({ page, request }) => {
        const subject = await deliverToInbox(page, request, 'BtnPhoneStar')
        await page.setViewportSize(PHONE)
        await phoneRow(page, subject).getByRole('button', { name: 'Add star', exact: true }).click()
        await expect(
            phoneRow(page, subject).getByRole('button', { name: 'Remove star', exact: true })
        ).toBeVisible()
    })

    test('the thread toolbar Labels sheet adds a label', async ({ page, request }) => {
        const label = await firstLabelName(page)
        await openFreshThread(page, request, 'BtnPhoneLabels')
        await page.setViewportSize(PHONE)
        await pressToolbarAction(page, await threadToolbar(page), 'Labels')
        await chooseMenuItem(page, label)
        await expect(visibleDetail(page).getByText(label, { exact: true })).toBeVisible()
    })

    test('the thread toolbar Move to sheet moves the thread', async ({ page, request }) => {
        await openFreshThread(page, request, 'BtnPhoneMove')
        await page.setViewportSize(PHONE)
        await pressToolbarAction(page, await threadToolbar(page), 'Move to')
        await chooseMenuItem(page, 'Archive')
        await expect(visibleDetail(page)).toHaveCount(0)
    })

    test('the thread toolbar star and read buttons work', async ({ page, request }) => {
        await openFreshThread(page, request, 'BtnPhoneStarRead')
        await page.setViewportSize(PHONE)
        const toolbar = await threadToolbar(page)
        // Which of these fit inline depends on the width, so each press finds
        // the flipped label wherever the toolbar put it.
        await pressToolbarAction(page, toolbar, 'Add star')
        await pressToolbarAction(page, toolbar, 'Remove star')
        await pressToolbarAction(page, toolbar, 'Mark as unread')
        await pressToolbarAction(page, toolbar, 'Mark as read')
    })

    test('More options on a message opens its sheet', async ({ page, request }) => {
        const subject = await openFreshThread(page, request, 'BtnPhoneMore')
        await page.setViewportSize(PHONE)
        await messageHeader(page).getByRole('button', { name: 'More options', exact: true }).click()
        await chooseMenuItem(page, 'Forward')
        await expect(page.getByTestId('mail-compose-subject')).toHaveValue(`Fwd: ${subject}`)
    })
})
