// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(cleanup)

const h = vi.hoisted(() => ({
    openHelp: vi.fn(),
    userId: 'u1',
    identities: [
        {
            id: 'i1',
            user: 'u1',
            email_name: 'nathan',
            inbox: 'mb1',
            default_from: '',
            retired: false,
        },
        { id: 'i2', user: 'u2', email_name: 'ann', inbox: 'mb2', default_from: '', retired: false },
        { id: 'i3', user: 'u3', email_name: 'old', inbox: '', default_from: '', retired: true },
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

test('the mail app link opens the IMAP help topic', async () => {
    const { findByText } = renderPage('u1')
    await act(async () => {
        fireEvent.click(await findByText('Set up a mail app'))
    })
    expect(h.openHelp).toHaveBeenCalledWith('mail:imap')
})
