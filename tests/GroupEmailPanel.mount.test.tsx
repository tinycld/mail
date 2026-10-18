// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render } from '@testing-library/react'
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
            ...localOnlyCollectionOptions({ id, getKey: (r: { id: string }) => r.id, initialData }),
            autoIndex: 'eager',
            defaultIndexType: BasicIndex,
        })
    const stores: Record<string, unknown> = {
        mail_domains: mk('mail_domains', h.domains),
        mail_mailboxes: mk('mail_mailboxes', h.mailboxes),
        mail_mailbox_members: mk('mail_mailbox_members', h.members),
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

test('domainsForGroup keeps rows listing the group, in rank order', () => {
    const rows = [
        { id: 'b', domain: 'b.example', coverageGroups: ['g'], rank: 2 },
        { id: 'a', domain: 'a.example', coverageGroups: ['g', 'h'], rank: 1 },
        { id: 'c', domain: 'c.example', coverageGroups: ['h'], rank: 0 },
    ]
    expect(domainsForGroup(rows, 'g').map(r => r.id)).toEqual(['a', 'b'])
})
