// @vitest-environment happy-dom
import { cleanup, render } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(cleanup)

// Mounts the real notice against real TanStack DB collections, so which
// addresses are lost comes out of the executed join, not fixtures handed to
// the pure helper.
const h = vi.hoisted(() => ({
    domains: [
        { id: 'd_q', domain: 'q.example', coverage: 'groups', coverage_groups: ['g_sales'] },
        {
            id: 'd_r',
            domain: 'r.example',
            coverage: 'groups',
            coverage_groups: ['g_sales', 'g_ops'],
        },
    ],
    users: [
        { id: 'ann', name: 'Ann', email: 'ann@tinycld.org' },
        { id: 'bob', name: 'Bob', email: 'bob@tinycld.org' },
        { id: 'cat', name: '', email: 'cat@tinycld.org' },
        { id: 'dan', name: 'Dan', email: 'dan@tinycld.org' },
        { id: 'eve', name: 'Eve', email: 'eve@tinycld.org' },
        { id: 'gus', name: 'Gus', email: 'gus@tinycld.org', role: 'guest' },
        { id: 'rob', name: 'Rob', email: 'rob@tinycld.org' },
    ],
    identities: [
        { id: 'i_bob', user: 'bob', inbox: 'mb_bob', email_name: 'bob', retired: false },
        { id: 'i_cat', user: 'cat', inbox: 'mb_cat', email_name: 'cat', retired: false },
        { id: 'i_ann', user: 'ann', inbox: 'mb_ann', email_name: 'ann', retired: false },
        { id: 'i_rob', user: 'rob', inbox: '', email_name: 'rob', retired: true },
    ],
    groups: [
        { id: 'g_sales', name: 'Sales' },
        { id: 'g_ops', name: 'Ops' },
    ],
    groupMembers: [
        { id: 'gm1', group: 'g_sales', user: 'ann' },
        { id: 'gm2', group: 'g_sales', user: 'cat' },
        // Cat is also in Ops, the other group of r.example: she keeps that address.
        { id: 'gm3', group: 'g_ops', user: 'cat' },
    ],
    mailboxes: [
        { id: 'mb_support', type: 'shared', address: 'support', domain: 'd_q' },
        { id: 'mb_ops', type: 'shared', address: 'ops', domain: 'd_q' },
    ],
    mailboxMembers: [
        // Grants of Sales: support only.
        { id: 'g1', mailbox: 'mb_support', group: 'g_sales', user: '', role: 'member' },
        { id: 'd1', mailbox: 'mb_support', group: 'g_sales', user: 'dan', role: 'member' },
        { id: 'd2', mailbox: 'mb_support', group: 'g_sales', user: 'eve', role: 'member' },
        // Eve also has a direct row: she keeps support.
        { id: 'd3', mailbox: 'mb_support', group: '', user: 'eve', role: 'member' },
        // Ops grants the other inbox.
        { id: 'g2', mailbox: 'mb_ops', group: 'g_ops', user: '', role: 'member' },
        { id: 'd4', mailbox: 'mb_ops', group: 'g_ops', user: 'dan', role: 'member' },
    ],
    addresses: [
        {
            id: 'a_ann_rule',
            local_part: 'ann',
            domain: 'd_q',
            mailbox: 'mb_ann',
            source: 'rule',
            is_primary: false,
            via_group: 'g_sales',
        },
        {
            id: 'a_ann_manual',
            local_part: 'sales',
            domain: 'd_q',
            mailbox: 'mb_ann',
            source: 'manual',
            is_primary: false,
            via_group: '',
        },
        {
            id: 'a_cat_q',
            local_part: 'cat',
            domain: 'd_q',
            mailbox: 'mb_cat',
            source: 'rule',
            is_primary: false,
            via_group: 'g_sales',
        },
        {
            id: 'a_cat_r',
            local_part: 'cat',
            domain: 'd_r',
            mailbox: 'mb_cat',
            source: 'rule',
            is_primary: false,
            via_group: 'g_sales',
        },
        {
            id: 'a_group',
            local_part: 'team',
            domain: 'd_q',
            mailbox: '',
            group: 'g_ops',
            source: 'manual',
            is_primary: false,
            via_group: '',
        },
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
        mail_identities: mk('mail_identities', h.identities),
        mail_addresses: mk('mail_addresses', h.addresses),
        users: mk('users', h.users),
        mail_mailboxes: mk('mail_mailboxes', h.mailboxes),
        mail_mailbox_members: mk('mail_mailbox_members', h.mailboxMembers),
        groups: mk('groups', h.groups),
        group_members: mk('group_members', h.groupMembers),
    }
    return { useStore: (...names: string[]) => names.map(n => stores[n]) }
})

import GroupRemovalNotice from '~/tinycld/mail/settings/GroupRemovalNotice'

test("removing a person lists only that person's rule addresses from the group", async () => {
    const { findByText } = render(<GroupRemovalNotice groupId="g_sales" userId="ann" />)
    expect(await findByText('Ann loses the address ann@q.example.')).toBeTruthy()
})

test('a person with no address from the group: the notice renders nothing', async () => {
    // Ann's notice beside Bob's proves the live queries ran: once hers shows,
    // the only text in the tree is hers, with no empty box for Bob.
    const { container, findByText } = render(
        <>
            <GroupRemovalNotice groupId="g_sales" userId="bob" />
            <GroupRemovalNotice groupId="g_sales" userId="ann" />
        </>
    )
    const ann = await findByText('Ann loses the address ann@q.example.')
    expect(container.childElementCount).toBe(1)
    expect(container.firstElementChild).toBe(ann)
})

test('an address another of the domain groups keeps is not listed', async () => {
    const { findByText } = render(<GroupRemovalNotice groupId="g_sales" userId="cat" />)
    // No name: the notice falls back to the email.
    expect(await findByText('cat@tinycld.org loses the address cat@q.example.')).toBeTruthy()
})

test('deleting the group lists every rule address its members lose', async () => {
    const { findByText } = render(<GroupRemovalNotice groupId="g_sales" userId={null} />)
    // Not cat@r.example: Cat stays in Ops, the other group of r.example.
    // Dan loses the support inbox too; Eve keeps it by her direct row.
    expect(
        await findByText(
            'Members lose 2 addresses: ann@q.example, cat@q.example. Members also lose access to support@q.example.'
        )
    ).toBeTruthy()
})

test('removing a person names the inbox only the group gave them', async () => {
    const { findByText } = render(<GroupRemovalNotice groupId="g_sales" userId="dan" />)
    expect(await findByText('Dan loses access to support@q.example.')).toBeTruthy()
})

test('an inbox the person also has directly is not listed', async () => {
    const { container, findByText } = render(
        <>
            <GroupRemovalNotice groupId="g_sales" userId="eve" />
            <GroupRemovalNotice groupId="g_sales" userId="dan" />
        </>
    )
    await findByText('Dan loses access to support@q.example.')
    expect(container.childElementCount).toBe(1)
})

test('deleting the group names the inboxes its members lose', async () => {
    const { findByText } = render(<GroupRemovalNotice groupId="g_ops" userId={null} />)
    expect(
        await findByText(
            'Members lose access to ops@q.example. The address team@q.example is deleted.'
        )
    ).toBeTruthy()
})

test('removing a person names the group address they stop getting', async () => {
    const { findByText } = render(<GroupRemovalNotice groupId="g_ops" userId="ann" />)
    expect(await findByText('Ann stops getting mail sent to team@q.example.')).toBeTruthy()
})

test('removing a guest names no group address: guests get no group mail', async () => {
    const { container, findByText } = render(
        <>
            <GroupRemovalNotice groupId="g_ops" userId="gus" />
            <GroupRemovalNotice groupId="g_ops" userId="ann" />
        </>
    )
    const ann = await findByText('Ann stops getting mail sent to team@q.example.')
    expect(container.childElementCount).toBe(1)
    expect(container.firstElementChild).toBe(ann)
})

test('removing a person with a retired identity names no group address', async () => {
    const { container, findByText } = render(
        <>
            <GroupRemovalNotice groupId="g_ops" userId="rob" />
            <GroupRemovalNotice groupId="g_ops" userId="ann" />
        </>
    )
    const ann = await findByText('Ann stops getting mail sent to team@q.example.')
    expect(container.childElementCount).toBe(1)
    expect(container.firstElementChild).toBe(ann)
})
