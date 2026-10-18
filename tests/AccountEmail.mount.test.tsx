// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(() => {
    cleanup()
    h.updated.length = 0
})

const h = vi.hoisted(() => ({
    openHelp: vi.fn(),
    push: vi.fn(),
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

vi.mock('expo-router', () => ({ useRouter: () => ({ push: h.push }) }))
vi.mock('@tinycld/core/lib/help/open-help', () => ({ openHelp: h.openHelp }))

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

import AccountEmail from '~/tinycld/mail/settings/AccountEmail'

function renderPage(userId: string) {
    h.userId = userId
    return render(
        <QueryClientProvider client={new QueryClient()}>
            <AccountEmail />
        </QueryClientProvider>
    )
}

test('lists the addresses read-only with the picker and the mail app link', async () => {
    const { findAllByText, getByText, queryByText, queryByLabelText } = renderPage('u1')
    expect((await findAllByText('nathan@tinycld.com')).length).toBeGreaterThan(0)
    expect(getByText('Main address')).toBeTruthy()
    expect(getByText('Send new mail from')).toBeTruthy()
    expect(getByText('Set up a mail app')).toBeTruthy()
    expect(queryByText('Add address')).toBeNull()
    expect(queryByLabelText('Remove address nathan@q.example')).toBeNull()
})

test('a person with an inbox and no address says so', async () => {
    const { findByText } = renderPage('u2')
    expect(await findByText('No address yet.')).toBeTruthy()
})

test('a guest sees that guests get no addresses', async () => {
    const { findByText, queryByText } = renderPage('u9')
    expect(await findByText('Guests do not get email addresses.')).toBeTruthy()
    expect(queryByText('Send new mail from')).toBeNull()
})

test('a member with no identity has no email yet', async () => {
    const { findByText, queryByText } = renderPage('u8')
    expect(await findByText('You have no email yet.')).toBeTruthy()
    expect(queryByText('Guests do not get email addresses.')).toBeNull()
})

test('the mail app link opens Settings → Account → Mail apps', async () => {
    const { findByText } = renderPage('u1')
    await act(async () => {
        fireEvent.click(await findByText('Set up a mail app'))
    })
    expect(h.push).toHaveBeenCalledWith('/a/settings/account/mail/mail-apps')
})

function switchFor(container: HTMLElement, label: string) {
    return container.querySelector<HTMLElement>(`[accessibilitylabel="${label}"]`)
}

async function press(el: HTMLElement | null) {
    if (!el) throw new Error('no element to press')
    await act(async () => {
        fireEvent.click(el)
    })
}

test('the address view switch writes address_views and shows a switch for each address', async () => {
    const { container, findAllByText, queryByText } = renderPage('u1')
    await findAllByText('nathan@tinycld.com')
    expect(switchFor(container, 'Show nathan@tinycld.com in sidebar')).toBeNull()
    expect(queryByText('Show in sidebar')).toBeNull()

    await press(switchFor(container, 'Show a folder for each address'))
    await waitFor(() => expect(h.updated).toHaveLength(1))
    expect(h.updated[0]).toMatchObject({ id: 'i1', address_views: true })

    await waitFor(() =>
        expect(switchFor(container, 'Show nathan@tinycld.com in sidebar')).toBeTruthy()
    )
    expect(switchFor(container, 'Show nathan@q.example in sidebar')).toBeTruthy()
    // The row switches share one visible column heading.
    expect(queryByText('Show in sidebar')).toBeTruthy()
})

test('turning off an address hides its folder', async () => {
    const { container, findAllByText } = renderPage('u4')
    await findAllByText('carl@q.example')
    await press(switchFor(container, 'Show carl@q.example in sidebar'))
    await waitFor(() => expect(h.updated).toHaveLength(1))
    expect(h.updated[0]).toMatchObject({ id: 'i4', hidden_address_views: ['a5'] })
})

test('with views on and one address left, the main switch is offered and turns views off', async () => {
    const { container, findAllByText, queryByText } = renderPage('u5')
    await findAllByText('dana@tinycld.com')
    const main = switchFor(container, 'Show a folder for each address')
    expect(main).toBeTruthy()
    expect(switchFor(container, 'Show dana@tinycld.com in sidebar')).toBeTruthy()
    await press(main)
    await waitFor(() => expect(h.updated).toHaveLength(1))
    expect(h.updated[0]).toMatchObject({ id: 'i5', address_views: false })
    // Off with one address: nothing more to turn on or off here.
    await waitFor(() => expect(switchFor(container, 'Show a folder for each address')).toBeNull())
    expect(switchFor(container, 'Show dana@tinycld.com in sidebar')).toBeNull()
    expect(queryByText('Show in sidebar')).toBeNull()
})
