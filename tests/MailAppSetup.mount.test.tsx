// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(() => {
    cleanup()
    h.updated.length = 0
    h.mailHost = 'mail.acme.example'
})

const h = vi.hoisted(() => ({
    openHelp: vi.fn(),
    mailHost: 'mail.acme.example' as string | null,
    userId: 'u1',
    updated: [] as Record<string, unknown>[],
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
vi.mock('react-native-svg', () => ({
    default: ({ children }: { children?: React.ReactNode }) => <div>{children}</div>,
    Path: () => <span />,
}))
vi.mock('@tinycld/core/lib/use-mail-host', () => ({ useMailHost: () => h.mailHost }))
vi.mock('@tinycld/core/file-viewer/use-authed-file-url', () => ({
    useFileToken: () => ({ data: 'file-token' }),
}))
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
    return { useStore: (...names: string[]) => names.map(n => stores[n]) }
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
    expect(
        container.querySelector('[accessibilitylabel="QR code that opens this page on your phone"]')
    ).not.toBeNull()
})

test('says so when the person has no address', async () => {
    h.userId = 'u8'
    const { findByText, byId } = renderSetup()
    expect(await findByText('You have no email address yet.')).toBeTruthy()
    expect(byId('mail-app-address')).toBeNull()
})

test('waits for the mail host', async () => {
    h.userId = 'u1'
    h.mailHost = null
    const { byId, rerender } = renderSetup()
    // The person's rows load from a local collection; give the live query time
    // to settle so a missing guard would have rendered the rows by now.
    await act(async () => {
        await new Promise(resolve => setTimeout(resolve, 50))
    })
    expect(byId('mail-app-imap-host')).toBeNull()

    h.mailHost = 'mail.acme.example'
    rerender(
        <QueryClientProvider client={new QueryClient()}>
            <MailAppSetup />
        </QueryClientProvider>
    )
    await waitFor(() => expect(byId('mail-app-imap-host')).not.toBeNull())
})
