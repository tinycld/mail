import * as fs from 'node:fs'
import * as path from 'node:path'
import { expect, type Page, test } from '@playwright/test'
import { clickSidebarItem, login, navigateToPackage } from '@tinycld/core/e2e-helpers'
import { emailRow, navigateToPersonalInbox, uniqueSubject } from './helpers'

// Attaching a file in compose and uploading it, through the real multipart
// path. Outbound SEND cannot be exercised here — providerForOrg returns an
// unconfigured PostmarkProvider without a server token, and the endpoint
// rejects on `!provider.Configured()` before any bytes move. The DRAFT
// endpoint takes the identical body over the identical transport and needs no
// provider, so it is what proves the upload works.
//
// The upload itself is XHR rather than fetch (fetch reports no progress), and
// that swap is the thing most likely to break silently: a broken transport
// still resolves, it just never delivers the bytes. Asserting the attachment
// survives a round trip to the server is what catches it.

const FIXTURE_PATH = path.join(import.meta.dirname, 'fixtures', 'hippo.jpg')

async function openCompose(page: Page) {
    await page.getByText('Compose', { exact: true }).click()
    // The compose drawer is a lazy chunk; first paint can exceed the default
    // timeout under load. Once "To" is up the rest is one render frame.
    await expect(page.getByText('To', { exact: true })).toBeVisible()
}

/**
 * Closes compose, which is what saves the draft (and therefore what POSTs the
 * multipart body). Escape does NOT do it while the body editor holds focus —
 * the keystroke goes to ProseMirror — so this drives the header button.
 */
async function closeCompose(page: Page) {
    await page.getByRole('button', { name: 'Close compose' }).click()
    await expect(page.getByText('To', { exact: true })).toBeHidden()
}

/** Attaches the fixture by driving the real picker. */
async function attachHippo(page: Page) {
    const chooserPromise = page.waitForEvent('filechooser')
    await page.getByRole('button', { name: 'Attach' }).click()
    const chooser = await chooserPromise
    await chooser.setFiles(FIXTURE_PATH)
}

test.describe('Mail — Compose attachments', () => {
    test.beforeEach(async ({ page }) => {
        await login(page)
        await navigateToPackage(page, 'mail', {
            waitFor: page.getByTestId('package-sidebar-mounted'),
        })
    })

    test('an attached file rides the draft upload to the server', async ({ page }) => {
        const subject = uniqueSubject('compose-attach')

        await openCompose(page)
        await page.getByTestId('mail-compose-subject').fill(subject)
        await page.locator('.tinycld-mail-editor .ProseMirror').click()
        await page.keyboard.type('Body text, so closing saves a draft.')

        await attachHippo(page)
        // The ribbon appears as soon as the file is held client-side, before
        // anything is uploaded — this is the "picked" half, not the transport.
        await expect(page.getByText('hippo.jpg', { exact: false }).first()).toBeVisible()

        // Closing with body text saves the draft, which is what POSTs the
        // multipart body over the XHR transport.
        await closeCompose(page)

        // The round trip is the assertion: the draft is only listed once the
        // server has accepted the upload and written the record.
        //
        // Personal mailbox first — compose saves into it, and the default
        // landing view is All Inboxes, whose Drafts is a different list.
        await navigateToPersonalInbox(page)
        await clickSidebarItem(page, 'Drafts')
        await expect(emailRow(page, subject)).toBeVisible({ timeout: 20_000 })

        // And the attachment survived it. A transport that resolved without
        // delivering bytes would still list the draft — this is the part that
        // would not survive.
        await emailRow(page, subject).click()
        await expect(page.getByTestId('mail-thread-detail')).toBeVisible({ timeout: 10_000 })
        await expect(page.getByText('hippo.jpg', { exact: false }).first()).toBeVisible({
            timeout: 15_000,
        })
    })

    test('no progress bar renders before anything is uploading', async ({ page }) => {
        await openCompose(page)
        await page.getByTestId('mail-compose-subject').fill(uniqueSubject('compose-idle'))

        await attachHippo(page)
        await expect(page.getByText('hippo.jpg', { exact: false }).first()).toBeVisible()

        // The file is held client-side until the message is sent or the draft
        // saved; a bar here would be reporting on nothing. This is the half of
        // the progress contract that IS reachable in e2e — see the note below
        // for why the in-flight half is not.
        await expect(page.getByTestId('mail-attachment-upload-progress')).toHaveCount(0)
    })
})

// NOT covered here, deliberately, so nobody assumes it is:
//
// The bar itself, mid-upload. It renders inside the compose window's ribbon,
// and only the SEND path keeps that window mounted while bytes move — the
// draft path calls close() immediately after firing saveDraft(), so its
// window unmounts before a single progress event lands. Send in turn needs a
// configured provider: providerForOrg returns an unconfigured
// PostmarkProvider without a server token and the endpoint rejects on
// `!provider.Configured()` before any bytes move, so an e2e send cannot get
// far enough to draw a bar.
//
// The bar's own behaviour is unit-covered in core (throttleProgress and the
// uploader's progress callback). Closing this gap needs a configured
// provider in the e2e environment — a fixture that stubs mail's Provider the
// way --dev already stubs core's mailer — which is a bigger change than this
// spec.

// The fixture is shared with mail-attachments.spec.ts; fail loudly rather
// than uploading an empty file if it ever goes missing.
if (!fs.existsSync(FIXTURE_PATH)) {
    throw new Error(`missing attachment fixture: ${FIXTURE_PATH}`)
}
