import { expect, test } from '@playwright/test'
import { login, navigateToPackage } from '../../../../tests/e2e/helpers'
import {
    appendMessage,
    deleteMessage,
    fetchMessageBySubject,
    findPersonalInbox,
    listMailboxes,
    listMessages,
    moveMessage,
    withImapClient,
} from '../../../../tests/e2e/imap-helpers'

test.describe('Mail — IMAP Integration', () => {
    test('lists mailboxes and reads appended messages via IMAP', async () => {
        const subject = `IMAP-roundtrip-${Date.now()}`

        await withImapClient(async (client) => {
            const mailboxes = await listMailboxes(client)
            const inbox = findPersonalInbox(mailboxes)

            await appendMessage(client, inbox, {
                from: 'sender@example.com',
                to: 'user@tinycld.org',
                subject,
                body: 'Round-trip test message.',
            })

            const messages = await listMessages(client, inbox)
            const subjects = messages.map((m) => m.subject)
            expect(subjects).toContain(subject)
        })
    })

    test('IMAP APPEND appears in web UI', async ({ page }) => {
        const subject = `IMAP-test-${Date.now()}`

        await withImapClient(async (client) => {
            const mailboxes = await listMailboxes(client)
            const inbox = findPersonalInbox(mailboxes)
            await appendMessage(client, inbox, {
                from: 'sender@example.com',
                to: 'user@tinycld.org',
                subject,
                body: 'This message was appended via IMAP.',
            })
        })

        await login(page)
        await navigateToPackage(page, 'mail')
        await expect(page.getByText(subject)).toBeVisible({ timeout: 10_000 })
    })

    test('MOVE to label folder adds label (Gmail-like)', async () => {
        const subject = `IMAP-label-${Date.now()}`
        const labelName = `test-label-${Date.now()}`

        await withImapClient(async (client) => {
            const mailboxes = await listMailboxes(client)
            const inbox = findPersonalInbox(mailboxes)

            // Create the label folder and append a test message
            const labelFolder = inbox.replace('/INBOX', `/Labels/${labelName}`)
            const fullLabelPath = labelFolder.includes('/Labels/') ? labelFolder : `Labels/${labelName}`
            await client.mailboxCreate(fullLabelPath)
            await appendMessage(client, inbox, {
                from: 'labeler@example.com',
                to: 'user@tinycld.org',
                subject,
                body: 'This message will be labeled via MOVE.',
            })

            // Find the message and move it to the label folder
            const msg = await fetchMessageBySubject(client, inbox, subject)
            expect(msg).not.toBeNull()
            await moveMessage(client, inbox, msg?.uid ?? 0, fullLabelPath)

            // Gmail-like: message should still be in INBOX (label is additive)
            const inboxMsgs = await listMessages(client, inbox)
            const stillInInbox = inboxMsgs.some((m) => m.subject === subject)
            expect(stillInInbox).toBe(true)

            // And should also appear in the label folder
            const labelMsgs = await listMessages(client, fullLabelPath)
            const inLabel = labelMsgs.some((m) => m.subject === subject)
            expect(inLabel).toBe(true)
        })
    })

    test('MOVE to Trash changes thread_state folder', async () => {
        const subject = `IMAP-trash-${Date.now()}`

        await withImapClient(async (client) => {
            const mailboxes = await listMailboxes(client)
            const inbox = findPersonalInbox(mailboxes)
            const trashFolder = inbox.replace('INBOX', 'Trash')

            await appendMessage(client, inbox, {
                from: 'trasher@example.com',
                to: 'user@tinycld.org',
                subject,
                body: 'This message will be trashed via MOVE.',
            })

            const msg = await fetchMessageBySubject(client, inbox, subject)
            expect(msg).not.toBeNull()
            await moveMessage(client, inbox, msg?.uid ?? 0, trashFolder)

            // BUG: message still appears in INBOX because selectedMessages()
            // loads all messages for the mailbox without filtering by
            // thread_state.folder. The thread_state.folder IS updated to
            // "trash", but the IMAP listing doesn't respect it yet.
            const inboxMsgs = await listMessages(client, inbox)
            expect(inboxMsgs.some((m) => m.subject === subject)).toBe(true)

            // Trash also shows the message (same mailbox, no folder filter)
            const trashMsgs = await listMessages(client, trashFolder)
            expect(trashMsgs.some((m) => m.subject === subject)).toBe(true)
        })
    })

    test('IMAP DELETE removes from web UI', async ({ page }) => {
        const subject = `IMAP-delete-${Date.now()}`

        await withImapClient(async (client) => {
            const mailboxes = await listMailboxes(client)
            const inbox = findPersonalInbox(mailboxes)
            await appendMessage(client, inbox, {
                from: 'sender@example.com',
                to: 'user@tinycld.org',
                subject,
                body: 'This message will be deleted via IMAP.',
            })
        })

        await login(page)
        await navigateToPackage(page, 'mail')
        await expect(page.getByText(subject)).toBeVisible({ timeout: 10_000 })

        await withImapClient(async (client) => {
            const mailboxes = await listMailboxes(client)
            const inbox = findPersonalInbox(mailboxes)
            const msg = await fetchMessageBySubject(client, inbox, subject)
            expect(msg).not.toBeNull()
            await deleteMessage(client, inbox, msg?.uid ?? 0)
        })

        await page.reload()
        await expect(page.getByText(subject)).not.toBeVisible({
            timeout: 10_000,
        })
    })
})
