import * as fs from 'node:fs'
import * as path from 'node:path'
import { type APIRequestContext, expect, type Locator, type Page, test } from '@playwright/test'
import PocketBase from 'pocketbase'
import { login, navigateToPackage } from '../../../../tests/e2e/helpers'

const PB_URL = 'http://127.0.0.1:7091'
const SUPERUSER_EMAIL = process.env.POCKETBASE_EMAIL || 'admin@tinycld.org'
const SUPERUSER_PASSWORD = process.env.POCKETBASE_PASSWORD || 'AdminPass1234!'

const FIXTURE_PATH = path.join(import.meta.dirname, 'fixtures', 'hippo.jpg')

function loadHippoFixture() {
    const buf = fs.readFileSync(FIXTURE_PATH)
    return { base64: buf.toString('base64'), byteLength: buf.byteLength }
}

async function getInboundWebhookUrl(): Promise<string> {
    const pb = new PocketBase(PB_URL)
    await pb.collection('_superusers').authWithPassword(SUPERUSER_EMAIL, SUPERUSER_PASSWORD)
    const domain = await pb
        .collection('mail_domains')
        .getFirstListItem<{ webhook_secret: string }>('domain = "tinycld.org"')
    if (!domain.webhook_secret) {
        throw new Error('mail_domains.webhook_secret is empty for tinycld.org — seed regression?')
    }
    return `${PB_URL}/api/mail/inbound/${domain.webhook_secret}`
}

interface InboundOptions {
    subject: string
    messageId: string
    base64: string
    byteLength: number
}

function buildPostmarkPayload({ subject, messageId, base64, byteLength }: InboundOptions) {
    return {
        From: 'sender@example.com',
        FromName: 'Hippo Sender',
        FromFull: { Name: 'Hippo Sender', Email: 'sender@example.com' },
        To: 'user@tinycld.org',
        ToFull: [{ Name: 'Test User', Email: 'user@tinycld.org' }],
        CcFull: [],
        Subject: subject,
        Date: new Date().toUTCString(),
        TextBody: 'Hello hippo',
        HtmlBody: '<p>Hello hippo:</p><p><img src="cid:hippo-inline" alt="hippo"></p>',
        StrippedTextReply: 'Hello hippo',
        MessageID: messageId,
        MailboxHash: '',
        Headers: [{ Name: 'Message-ID', Value: `<${messageId}>` }],
        Attachments: [
            {
                Name: 'hippo.jpg',
                Content: base64,
                ContentType: 'image/jpeg',
                ContentID: 'cid:hippo-inline',
                ContentLength: byteLength,
            },
            {
                Name: 'hippo.jpg',
                Content: base64,
                ContentType: 'image/jpeg',
                ContentID: '',
                ContentLength: byteLength,
            },
        ],
    }
}

async function deliverInbound(request: APIRequestContext, url: string, payload: object) {
    const res = await request.post(url, {
        data: payload,
        headers: { 'Content-Type': 'application/json' },
    })
    if (!res.ok()) {
        const body = await res.text()
        throw new Error(`Inbound delivery failed: ${res.status()} ${body}`)
    }
}

async function expectImageLoaded(target: Locator) {
    await expect
        .poll(async () => target.evaluate((el: HTMLImageElement) => el.complete && el.naturalWidth > 0), {
            timeout: 5_000,
        })
        .toBe(true)
}

function inlineBodyImageLocator(page: Page): Locator {
    return page.frameLocator('iframe[title="Email body"]').locator('img[alt="hippo"]')
}

test.describe('Mail — Attachments', () => {
    test('inline + attached image render after Postmark inbound delivery', async ({ page, request }) => {
        const { base64, byteLength } = loadHippoFixture()
        const stamp = Date.now()
        const subject = `Attachments regression — hippo ${stamp}`
        const messageId = `attachment-test-${stamp}@tinycld.test`

        const inboundUrl = await getInboundWebhookUrl()
        await deliverInbound(request, inboundUrl, buildPostmarkPayload({ subject, messageId, base64, byteLength }))

        await login(page)
        await navigateToPackage(page, 'mail')

        await expect(page.getByText(subject).first()).toBeVisible({ timeout: 10_000 })
        await page.getByText(subject).first().click()

        const inlineImg = inlineBodyImageLocator(page)
        await expect(inlineImg).toBeVisible()
        await expectImageLoaded(inlineImg)

        // Attachment ribbon thumbnail: <img> served from PocketBase's files
        // endpoint. The filename gets a 10-char random suffix, so match the
        // path prefix and extension rather than the literal filename.
        const thumb = page.locator('img[src*="/api/files/"][src*="hippo"][src$=".jpg"]').first()
        await expect(thumb).toBeVisible()
        await expectImageLoaded(thumb)
    })
})
