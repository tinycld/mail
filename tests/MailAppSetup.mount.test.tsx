// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import { qrPath } from '@tinycld/core/components/settings/security/qr-matrix'
import { log } from '@tinycld/core/lib/logger'
import { Linking } from 'react-native'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'
import { setupPageUrl } from '~/tinycld/mail/settings/mail-app-setup'

afterEach(() => {
    cleanup()
    h.updated.length = 0
    h.mailHost = 'mail.acme.example'
    vi.restoreAllMocks()
    h.send.mockReset()
})

beforeEach(() => {
    h.send.mockResolvedValue({ token: 'profile-token' })
})

const h = vi.hoisted(() => ({
    openHelp: vi.fn(),
    mailHost: 'mail.acme.example' as string | null,
    userId: 'u1',
    updated: [] as Record<string, unknown>[],
    send: vi.fn(),
    identities: [
        {
            id: 'i1',
            user: 'u1',
            email_name: 'nathan',
            inbox: 'mb1',
            default_from: '',
            retired: false,
            address_views: false,
            hidden_address_views: [] as string[],
        },
        {
            id: 'i2',
            user: 'u2',
            email_name: 'ann',
            inbox: 'mb2',
            default_from: '',
            retired: false,
            address_views: false,
            hidden_address_views: [] as string[],
        },
        {
            id: 'i3',
            user: 'u3',
            email_name: 'old',
            inbox: '',
            default_from: '',
            retired: true,
            address_views: false,
            hidden_address_views: [] as string[],
        },
        {
            id: 'i4',
            user: 'u4',
            email_name: 'carl',
            inbox: 'mb4',
            default_from: '',
            retired: false,
            address_views: true,
            hidden_address_views: [] as string[],
        },
        {
            id: 'i5',
            user: 'u5',
            email_name: 'dana',
            inbox: 'mb5',
            default_from: '',
            retired: false,
            address_views: true,
            hidden_address_views: [] as string[],
        },
    ],
    addresses: [
        {
            id: 'a1',
            local_part: 'nathan',
            domain: 'd1',
            mailbox: 'mb1',
            source: 'rule',
            is_primary: true,
            via_group: '',
        },
        {
            id: 'a2',
            local_part: 'nathan',
            domain: 'd2',
            mailbox: 'mb1',
            source: 'manual',
            is_primary: false,
            via_group: '',
        },
        {
            id: 'a4',
            local_part: 'carl',
            domain: 'd1',
            mailbox: 'mb4',
            source: 'rule',
            is_primary: true,
            via_group: '',
        },
        {
            id: 'a5',
            local_part: 'carl',
            domain: 'd2',
            mailbox: 'mb4',
            source: 'manual',
            is_primary: false,
            via_group: '',
        },
        {
            id: 'a6',
            local_part: 'dana',
            domain: 'd1',
            mailbox: 'mb5',
            source: 'rule',
            is_primary: true,
            via_group: '',
        },
    ],
    domains: [
        { id: 'd1', domain: 'tinycld.com', rank: 1 },
        { id: 'd2', domain: 'q.example', rank: 2 },
    ],
    mailboxes: [
        { id: 'mb1', type: 'personal', display_name: 'Nathan' },
        { id: 'mb_sales', type: 'shared', display_name: 'Sales inbox' },
        { id: 'mb_blank', type: 'shared', display_name: '', name: 'Support', address: 'support' },
        { id: 'mb_other', type: 'shared', display_name: 'Other inbox' },
    ],
    users: [
        { id: 'u1', role: 'admin' },
        { id: 'u2', role: 'member' },
        { id: 'u3', role: 'member' },
        { id: 'u4', role: 'member' },
        { id: 'u5', role: 'member' },
        { id: 'u8', role: 'member' },
        { id: 'u9', role: 'guest' },
    ],
    members: [
        { id: 'm1', mailbox: 'mb_sales', user: 'u1' },
        { id: 'm2', mailbox: 'mb1', user: 'u1' },
        { id: 'm4', mailbox: 'mb_blank', user: 'u1' },
        { id: 'm3', mailbox: 'mb_other', user: 'u9' },
    ],
}))

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: { id: h.userId }, isLoggedIn: true }),
}))

vi.mock('@tinycld/core/lib/help/open-help', () => ({ openHelp: h.openHelp }))
// The real QrCode renders; the SVG mock exposes the path it draws, so a test can
// compare the code with the one core makes for a URL.
vi.mock('react-native-svg', () => ({
    default: ({ children }: { children?: React.ReactNode }) => <div>{children}</div>,
    Path: ({ d }: { d: string }) => <span data-d={d} />,
}))
vi.mock('@tinycld/core/lib/use-mail-host', () => ({ useMailHost: () => h.mailHost }))
vi.mock('@tinycld/core/lib/server-address', () => ({
    getResolvedAddress: () => 'https://acme.tinycld.org',
}))

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { BasicIndex, createCollection, localOnlyCollectionOptions } = await import(
        '@tanstack/db'
    )
    const mk = (id: string, initialData: { id: string }[]) =>
        createCollection({
            ...localOnlyCollectionOptions({
                id,
                getKey: (r: { id: string }) => r.id,
                initialData,
                onUpdate: async ({ transaction }) => {
                    for (const m of transaction.mutations) h.updated.push(m.modified)
                },
            }),
            autoIndex: 'eager',
            defaultIndexType: BasicIndex,
        })
    const stores: Record<string, unknown> = {
        mail_identities: mk('mail_identities', h.identities),
        mail_addresses: mk('mail_addresses', h.addresses),
        mail_domains: mk('mail_domains', h.domains),
        groups: mk('groups', []),
        users: mk('users', h.users),
        mail_mailboxes: mk('mail_mailboxes', h.mailboxes),
        mail_mailbox_members: mk('mail_mailbox_members', h.members),
    }
    return {
        useStore: (...names: string[]) => names.map(n => stores[n]),
        pb: { send: h.send, authStore: { isValid: true } },
    }
})

import MailAppSetup from '~/tinycld/mail/settings/MailAppSetup'

// react-native-web under happy-dom renders testID as a plain `testid` attribute.
function renderSetup() {
    const ui = render(
        <QueryClientProvider client={new QueryClient()}>
            <MailAppSetup />
        </QueryClientProvider>
    )
    const byId = (id: string) => ui.container.querySelector(`[testid="${id}"]`)
    return {
        ...ui,
        byId,
        findById: (id: string) =>
            waitFor(() => expect(byId(id)).not.toBeNull()).then(() => byId(id)),
    }
}

test('shows the settings for the Default From address', async () => {
    h.userId = 'u1'
    const { container, findById } = renderSetup()
    expect((await findById('mail-app-address'))?.textContent).toBe('nathan@tinycld.com')
    expect((await findById('mail-app-user-name'))?.textContent).toBe('nathan@tinycld.com')
    expect((await findById('mail-app-imap-host'))?.textContent).toBe('mail.acme.example')
    expect((await findById('mail-app-smtp-host'))?.textContent).toBe('mail.acme.example')
    expect(container.querySelector(QR_LABEL)).not.toBeNull()
})

// react-native-web under happy-dom renders accessibilityLabel as a plain lower-case attribute.
const QR_LABEL = '[accessibilitylabel="QR code that opens this page on your phone"]'

test('the QR code draws the set-up page and holds no token', async () => {
    h.userId = 'u1'
    const { container, findById, byId } = renderSetup()
    await findById('mail-app-address')
    const drawn = container.querySelector(`${QR_LABEL} [data-d]`)?.getAttribute('data-d')
    const pageUrl = setupPageUrl('https://acme.tinycld.org')
    expect(drawn).toBe(qrPath(pageUrl).path)
    expect(drawn).not.toBe(qrPath(`${pageUrl}?token=profile-token`).path)
    await waitFor(() =>
        expect(byId('mail-app-profile-button')?.getAttribute('data-disabled')).toBe('false')
    )
})

test('tells a guest that guests get no address', async () => {
    h.userId = 'u9'
    const { findByText, queryByText } = renderSetup()
    expect(await findByText('Guests do not get email addresses.')).toBeTruthy()
    expect(queryByText('You have no email address yet.')).toBeNull()
})

test('says so when the person has no address', async () => {
    h.userId = 'u8'
    const { findByText, byId } = renderSetup()
    expect(await findByText('You have no email address yet.')).toBeTruthy()
    expect(byId('mail-app-address')).toBeNull()
})

test('hides the host rows when the mail host becomes unknown', async () => {
    h.userId = 'u1'
    const { byId, findById, rerender } = renderSetup()
    // The host row shows once the person's data has loaded, so the data is in
    // the store when the host goes back to pending below.
    await findById('mail-app-imap-host')

    h.mailHost = null
    rerender(
        <QueryClientProvider client={new QueryClient()}>
            <MailAppSetup />
        </QueryClientProvider>
    )
    expect(byId('mail-app-imap-host')).toBeNull()
    expect(byId('mail-app-address')).toBeNull()
})

async function pressProfileButton(findById: (id: string) => Promise<Element | null>) {
    const button = await findById('mail-app-profile-button')
    await waitFor(() => expect(button?.getAttribute('data-disabled')).toBe('false'))
    fireEvent.click(button as Element)
}

test('the profile link holds the profile token fetched from the server', async () => {
    h.userId = 'u1'
    const open = vi.spyOn(Linking, 'openURL').mockResolvedValue(undefined)
    const { findById } = renderSetup()
    await pressProfileButton(findById)
    expect(h.send).toHaveBeenCalledWith('/api/mail/mobileconfig/token', { method: 'POST' })
    expect(open).toHaveBeenCalledWith(
        'https://acme.tinycld.org/api/mail/mobileconfig?token=profile-token'
    )
})

test('logs a failed open of the profile link and does not throw', async () => {
    h.userId = 'u1'
    const failure = new Error('blocked')
    vi.spyOn(Linking, 'openURL').mockRejectedValue(failure)
    const error = vi.spyOn(log, 'error').mockImplementation(() => undefined)
    const { findById } = renderSetup()
    await pressProfileButton(findById)
    await waitFor(() =>
        expect(error).toHaveBeenCalledWith('mail.client-setup', failure, { url: 'profile' })
    )
})

const MINUTE = 60_000

test('an old profile token is fetched again before the link opens', async () => {
    h.userId = 'u1'
    vi.useFakeTimers({ toFake: ['Date'] })
    const open = vi.spyOn(Linking, 'openURL').mockResolvedValue(undefined)
    const { findById } = renderSetup()
    const button = await findById('mail-app-profile-button')
    await waitFor(() => expect(button?.getAttribute('data-disabled')).toBe('false'))
    expect(h.send).toHaveBeenCalledTimes(1)

    h.send.mockResolvedValue({ token: 'fresh-token' })
    vi.setSystemTime(Date.now() + 10 * MINUTE)
    fireEvent.click(button as Element)

    await waitFor(() =>
        expect(open).toHaveBeenCalledWith(
            'https://acme.tinycld.org/api/mail/mobileconfig?token=fresh-token'
        )
    )
    expect(h.send).toHaveBeenCalledTimes(2)
    vi.useRealTimers()
})

test('a fresh profile token opens the link with no new fetch', async () => {
    h.userId = 'u1'
    vi.useFakeTimers({ toFake: ['Date'] })
    const open = vi.spyOn(Linking, 'openURL').mockResolvedValue(undefined)
    const { findById } = renderSetup()
    const button = await findById('mail-app-profile-button')
    await waitFor(() => expect(button?.getAttribute('data-disabled')).toBe('false'))

    vi.setSystemTime(Date.now() + 8 * MINUTE)
    fireEvent.click(button as Element)

    expect(open).toHaveBeenCalledWith(
        'https://acme.tinycld.org/api/mail/mobileconfig?token=profile-token'
    )
    expect(h.send).toHaveBeenCalledTimes(1)
    vi.useRealTimers()
})

test('logs a failed refetch of an old token and does not open the link', async () => {
    h.userId = 'u1'
    vi.useFakeTimers({ toFake: ['Date'] })
    const open = vi.spyOn(Linking, 'openURL').mockResolvedValue(undefined)
    const warn = vi.spyOn(log, 'warn').mockImplementation(() => undefined)
    const { findById } = renderSetup()
    const button = await findById('mail-app-profile-button')
    await waitFor(() => expect(button?.getAttribute('data-disabled')).toBe('false'))

    const failure = new Error('offline')
    h.send.mockRejectedValue(failure)
    vi.setSystemTime(Date.now() + 10 * MINUTE)
    fireEvent.click(button as Element)

    await waitFor(() =>
        expect(warn).toHaveBeenCalledWith('mail.client-setup', 'profile token fetch failed', {
            error: String(failure),
        })
    )
    expect(open).not.toHaveBeenCalled()
    vi.useRealTimers()
})

test('says the profile is not available when the token fetch fails', async () => {
    h.userId = 'u1'
    const warn = vi.spyOn(log, 'warn').mockImplementation(() => undefined)
    const failure = new Error('offline')
    h.send.mockRejectedValue(failure)
    const { findByText, findById } = renderSetup()
    expect(await findByText('The profile is not available. Try again later.')).toBeTruthy()
    expect((await findById('mail-app-profile-button'))?.getAttribute('data-disabled')).toBe('true')
    expect(warn).toHaveBeenCalledTimes(1)
    expect(warn).toHaveBeenCalledWith('mail.client-setup', 'profile token fetch failed', {
        error: String(failure),
    })
})
