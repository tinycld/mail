// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(cleanup)

const h = vi.hoisted(() => ({
    ready: true,
    domains: [
        {
            id: 'd_q',
            domain: 'q.example',
            coverage: 'groups',
            coverage_groups: ['g_sales'],
            rank: 1,
        },
        { id: 'd_r', domain: 'r.example', coverage: 'groups', coverage_groups: ['g_ops'], rank: 2 },
        {
            id: 'd_e',
            domain: 'e.example',
            coverage: 'everyone',
            coverage_groups: ['g_sales'],
            rank: 3,
        },
    ],
    addresses: [
        { id: 'ga1', local_part: 'sales', domain: 'd_q', mailbox: '', group: 'g_sales' },
        { id: 'ga2', local_part: 'ops', domain: 'd_r', mailbox: '', group: 'g_ops' },
        { id: 'pa1', local_part: 'ann', domain: 'd_q', mailbox: 'mb_ann', group: '' },
    ],
    identities: [{ id: 'i1', email_name: 'holly', inbox: 'mb_h', user: 'u_h', retired: false }],
    inserted: [] as Record<string, unknown>[],
    deleted: [] as string[],
    mailboxes: [{ id: 'mb_support', type: 'shared', address: 'support', domain: 'd_q' }],
    members: [
        { id: 'g1', mailbox: 'mb_support', group: 'g_sales', user: '', role: 'member' },
        // A derived row is not a grant.
        { id: 'd1', mailbox: 'mb_support', group: 'g_sales', user: 'ann', role: 'member' },
        { id: 'g2', mailbox: 'mb_support', group: 'g_ops', user: '', role: 'member' },
    ],
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
                onInsert: async ({ transaction }) => {
                    for (const m of transaction.mutations) h.inserted.push(m.modified)
                },
                onDelete: async ({ transaction }) => {
                    for (const m of transaction.mutations) h.deleted.push(String(m.key))
                },
            }),
            autoIndex: 'eager',
            defaultIndexType: BasicIndex,
        })
    const stores: Record<string, unknown> = {
        mail_domains: mk('mail_domains', h.domains),
        mail_mailboxes: mk('mail_mailboxes', h.mailboxes),
        mail_mailbox_members: mk('mail_mailbox_members', h.members),
        mail_addresses: mk('mail_addresses', h.addresses),
        mail_identities: mk('mail_identities', h.identities),
    }
    // A collection whose first load never finishes.
    const loading = createCollection({
        id: 'mail_domains_loading',
        getKey: (r: { id: string }) => r.id,
        sync: { sync: () => undefined },
    })
    return {
        useStore: (...names: string[]) => names.map(n => (h.ready ? stores[n] : loading)),
    }
})

import GroupEmailPanel from '~/tinycld/mail/settings/GroupEmailPanel'
import { domainsForGroup } from '~/tinycld/mail/settings/group-domains'

function renderPanel(groupId: string) {
    return render(
        <QueryClientProvider client={new QueryClient()}>
            <GroupEmailPanel groupId={groupId} />
        </QueryClientProvider>
    )
}

test('lists the domains whose coverage includes the group', async () => {
    const { findByText, queryByText } = renderPanel('g_sales')
    expect(await findByText('Email')).toBeTruthy()
    expect(await findByText('Gives addresses on q.example')).toBeTruthy()
    expect(queryByText(/r\.example/)).toBeNull()
    expect(queryByText(/e\.example/)).toBeNull()
})

test('lists the inboxes the group grants', async () => {
    const { findByText, queryByText } = renderPanel('g_sales')
    expect(await findByText('Can use support@q.example')).toBeTruthy()
    expect(queryByText(/Can use .*ann/)).toBeNull()
})

test('lists no inbox for a group that grants none', async () => {
    const { findByText, queryByText } = renderPanel('g_none')
    await findByText('This group gives no addresses.')
    expect(queryByText(/Can use/)).toBeNull()
})

test('says so when the group gives no addresses', async () => {
    const { findByText } = renderPanel('g_none')
    expect(await findByText('This group gives no addresses.')).toBeTruthy()
})

test('says nothing about addresses before the domains have loaded', async () => {
    h.ready = false
    const { findByText, queryByText } = renderPanel('g_none')
    expect(await findByText('Email')).toBeTruthy()
    expect(queryByText('This group gives no addresses.')).toBeNull()
    h.ready = true
})

test("lists the group's own addresses and not other groups' or people's", async () => {
    const { findByText, queryByText } = renderPanel('g_sales')
    expect(await findByText('Group addresses')).toBeTruthy()
    expect(await findByText('sales@q.example')).toBeTruthy()
    expect(queryByText('ops@r.example')).toBeNull()
    expect(queryByText('ann@q.example')).toBeNull()
    expect(
        await findByText('Mail to these addresses goes to every member of the group.')
    ).toBeTruthy()
})

test('a group with no address of its own says so', async () => {
    const { findByText } = renderPanel('g_none')
    expect(await findByText('This group has no address yet.')).toBeTruthy()
})

test('a group with an address does not show the empty line', async () => {
    const { findByText, queryByText } = renderPanel('g_sales')
    await findByText('sales@q.example')
    expect(queryByText('This group has no address yet.')).toBeNull()
})

test('removes a group address', async () => {
    h.deleted.length = 0
    const { container, findByText } = renderPanel('g_sales')
    await findByText('sales@q.example')
    const button = container.querySelector('[accessibilitylabel="Remove address sales@q.example"]')
    if (!button) throw new Error('no remove button rendered')
    await act(async () => {
        fireEvent.click(button)
    })
    await waitFor(() => expect(h.deleted).toEqual(['ga1']))
})

test('adds a group address through the form', async () => {
    h.inserted.length = 0
    const { findByText, container } = renderPanel('g_sales')
    expect(container.querySelector('rn-textinput')).toBeNull()
    await act(async () => {
        fireEvent.click(await findByText('Add address'))
    })
    const input = container.querySelector<HTMLInputElement>('rn-textinput')
    if (!input) throw new Error('no input rendered')
    await act(async () => {
        fireEvent.input(input, { target: { value: 'team' } })
    })
    await act(async () => {
        fireEvent.click(await findByText('q.example'))
    })
    await waitFor(() => expect(container.querySelector('[aria-disabled="true"]')).toBeNull())
    await act(async () => {
        fireEvent.click(await findByText('Add address'))
    })
    await waitFor(() => expect(h.inserted).toHaveLength(1))
    expect(h.inserted[0]).toMatchObject({
        local_part: 'team',
        domain: 'd_q',
        mailbox: '',
        group: 'g_sales',
    })
})

test('domainsForGroup keeps rows listing the group, in rank order', () => {
    const rows = [
        { id: 'b', domain: 'b.example', coverageGroups: ['g'], rank: 2 },
        { id: 'a', domain: 'a.example', coverageGroups: ['g', 'h'], rank: 1 },
        { id: 'c', domain: 'c.example', coverageGroups: ['h'], rank: 0 },
    ]
    expect(domainsForGroup(rows, 'g').map(r => r.id)).toEqual(['a', 'b'])
})
