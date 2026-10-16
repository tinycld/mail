// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(cleanup)

const h = vi.hoisted(() => ({
    openHelp: vi.fn(),
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
    members: [
        { id: 'm1', mailbox: 'mb_sales', user: 'u1' },
        { id: 'm2', mailbox: 'mb1', user: 'u1' },
        { id: 'm4', mailbox: 'mb_blank', user: 'u1' },
        { id: 'm3', mailbox: 'mb_other', user: 'u9' },
    ],
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
        mail_mailboxes: mk('mail_mailboxes', h.mailboxes),
        mail_mailbox_members: mk('mail_mailbox_members', h.members),
    }
    return { useStore: (...names: string[]) => names.map(n => stores[n]) }
})

import MemberEmailPanel from '~/tinycld/mail/settings/MemberEmailPanel'

function renderPanel(userId: string) {
    return render(
        <QueryClientProvider client={new QueryClient()}>
            <MemberEmailPanel userId={userId} />
        </QueryClientProvider>
    )
}

test('shows the heading, addresses with badges, the email name and the picker', async () => {
    const { findByText, findAllByText, getByText } = renderPanel('u1')
    expect(await findByText('Email')).toBeTruthy()
    // Also an option of the Default From picker.
    expect((await findAllByText('nathan@tinycld.com')).length).toBeGreaterThan(0)
    expect(getByText('Main address')).toBeTruthy()
    expect(getByText('Main address')).toBeTruthy()
    expect(getByText('nathan')).toBeTruthy()
    expect(getByText('Edit')).toBeTruthy()
    expect(getByText('Send new mail from')).toBeTruthy()
})

test('Edit shows the rename form', async () => {
    const { findByText } = renderPanel('u1')
    await act(async () => {
        fireEvent.click(await findByText('Edit'))
    })
    expect(await findByText('Rename')).toBeTruthy()
})

test('lists the shared inboxes of the person', async () => {
    const { findByText, queryByText } = renderPanel('u1')
    expect(await findByText('Sales inbox')).toBeTruthy()
    expect(queryByText('Other inbox')).toBeNull()
})

test('a shared inbox with an empty display name shows its name', async () => {
    const { findByText } = renderPanel('u1')
    expect(await findByText('Support')).toBeTruthy()
})

test('no shared inbox list when there are none', async () => {
    const { findByText, queryByText } = renderPanel('u2')
    await findByText('Send new mail from')
    expect(queryByText('Shared inboxes')).toBeNull()
})

test('a person with an inbox and no address says so', async () => {
    const { findByText } = renderPanel('u2')
    expect(await findByText('No address yet.')).toBeTruthy()
})

test('a guest has no identity and no form', async () => {
    const { findByText, queryByText } = renderPanel('u9')
    expect(await findByText('Guests do not get email addresses.')).toBeTruthy()
    expect(queryByText('Send new mail from')).toBeNull()
    expect(queryByText('Edit')).toBeNull()
})

test('a retired identity says the person has left', async () => {
    const { findByText, queryByText } = renderPanel('u3')
    expect(await findByText('This person has left. Their addresses are removed.')).toBeTruthy()
    expect(queryByText('Send new mail from')).toBeNull()
})

test('the mail app link opens the IMAP help topic', async () => {
    const { findByText } = renderPanel('u1')
    await act(async () => {
        fireEvent.click(await findByText('Set up a mail app'))
    })
    expect(h.openHelp).toHaveBeenCalledWith('mail:imap')
})
