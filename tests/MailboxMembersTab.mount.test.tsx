// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(cleanup)

const h = vi.hoisted(() => ({
    stores: {} as Record<string, unknown>,
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
    h.stores.mail_mailbox_members = mk('mail_mailbox_members', [
        { id: 'grant_ops', mailbox: 'mb1', user: '', group: 'g_ops', role: 'member' },
    ])
    h.stores.groups = mk('groups', [
        { id: 'g_ops', name: 'Ops', description: '' },
        { id: 'g_sales', name: 'Sales', description: '' },
    ])
    h.stores.group_members = mk('group_members', [])
    return { useStore: (...names: string[]) => names.map(n => h.stores[n]) }
})

import { OverlayProvider } from '@tinycld/core/ui/overlay'
import { MembersTab } from '~/tinycld/mail/settings/MailboxDrawer'
import { foldMembers, type MemberRow, viaGroupLabel } from '~/tinycld/mail/settings/member-rows'

function row(over: Partial<MemberRow>): MemberRow {
    return {
        id: 'r',
        userId: 'u',
        userName: 'U',
        userEmail: '',
        role: 'member',
        isYou: false,
        groupId: null,
        groupNames: [],
        ...over,
    }
}

const orgMembers = [
    { userId: 'me', userName: 'Me', userEmail: '' },
    { userId: 'ann', userName: 'Ann', userEmail: '' },
    { userId: 'zed', userName: 'Zed', userEmail: '' },
]

function mountDrawer(members: MemberRow[]) {
    return render(
        <QueryClientProvider client={new QueryClient()}>
            <OverlayProvider>
                <MembersTab mailboxId="mb1" members={members} orgMembers={orgMembers} canWrite />
            </OverlayProvider>
        </QueryClientProvider>
    )
}

const me = row({ id: 'm_me', userId: 'me', userName: 'Me', role: 'owner', isYou: true })
const derived = row({
    id: 'm_ann',
    userId: 'ann',
    userName: 'Ann',
    groupId: 'g_sales',
    groupNames: ['Sales'],
})
// The group row is not loaded, or the viewer cannot read groups: the row is
// still group access, known from its group id.
const derivedNoName = row({
    id: 'm_ann',
    userId: 'ann',
    userName: 'Ann',
    groupId: 'g_hidden',
})

// react-native-web under happy-dom renders testID as a plain `testid` attribute.
async function findByTestId(container: HTMLElement, testId: string) {
    return await waitFor(() => {
        const found = container.querySelector(`[testid="${testId}"]`)
        if (!found) throw new Error(`no element with testID ${testId}`)
        return found as HTMLElement
    })
}

test('the members tab shows the Groups section with the existing grant', async () => {
    const { findByText, container } = mountDrawer([me])
    expect(await findByTestId(container, 'group-share-section')).toBeTruthy()
    expect(await findByText('Groups')).toBeTruthy()
    expect(await findByTestId(container, 'group-grant-row-g_ops')).toBeTruthy()
})

test('adding a group inserts a member-role grant row for the inbox', async () => {
    const { findByText, container } = mountDrawer([me])
    fireEvent.click(await findByText('Add group'))
    const pickerRow = await findByTestId(container, 'group-picker-row-g_sales')
    fireEvent.click(within(pickerRow).getByText('Add'))
    const collection = h.stores.mail_mailbox_members as {
        toArray: { group: string; user: string; mailbox: string; role: string }[]
    }
    await waitFor(() =>
        expect(collection.toArray).toContainEqual(
            expect.objectContaining({ mailbox: 'mb1', group: 'g_sales', user: '', role: 'member' })
        )
    )
})

test('a person who only has access through a group shows the group and no remove control', async () => {
    const { findByText, container } = mountDrawer([me, derived])
    expect(await findByText('Through group Sales')).toBeTruthy()
    expect(container.querySelector('[accessibilitylabel="Remove Ann"]')).toBeNull()
    expect(container.querySelector('[accessibilitylabel="Remove Me"]')).not.toBeNull()
})

test('a person with only group access is offered in the picker and gets a direct row', async () => {
    const { findByText, queryAllByText } = mountDrawer([me, derived])
    fireEvent.click(await findByText('Add member'))
    expect(await findByText('Zed')).toBeTruthy()
    // Ann: once in the member list, once as a picker choice. Me has a direct row: list only.
    expect(queryAllByText('Ann')).toHaveLength(2)
    expect(queryAllByText('Me')).toHaveLength(1)
    const choice = queryAllByText('Ann')[1]
    fireEvent.click(choice)
    fireEvent.click(await findByText('Add'))
    const collection = h.stores.mail_mailbox_members as {
        toArray: { group: string; user: string; mailbox: string; role: string }[]
    }
    await waitFor(() =>
        expect(collection.toArray).toContainEqual(
            expect.objectContaining({ mailbox: 'mb1', user: 'ann', group: '', role: 'member' })
        )
    )
})

test('a group row that is not loaded still shows as group access', async () => {
    const { findByText, queryAllByText, container } = mountDrawer([me, derivedNoName])
    expect(await findByText('Through a group')).toBeTruthy()
    expect(container.querySelector('[accessibilitylabel="Remove Ann"]')).toBeNull()
    fireEvent.click(await findByText('Add member'))
    expect(await findByText('Zed')).toBeTruthy()
    expect(queryAllByText('Ann')).toHaveLength(2)
})

test('a person with access through two groups names both groups', async () => {
    const both = foldMembers([
        derived,
        row({
            id: 'm_ann2',
            userId: 'ann',
            userName: 'Ann',
            groupId: 'g_ops',
            groupNames: ['Ops'],
        }),
    ])
    const { findByText } = mountDrawer([me, ...both])
    expect(await findByText('Through groups Sales, Ops')).toBeTruthy()
})

test('foldMembers: one row per person, the direct row wins, group-only rows name their groups', () => {
    const rows = [
        row({
            id: 'a1',
            userId: 'ann',
            userName: 'Ann',
            groupId: 'g_sales',
            groupNames: ['Sales'],
        }),
        row({ id: 'a2', userId: 'ann', userName: 'Ann', role: 'owner' }),
        row({
            id: 'b1',
            userId: 'bob',
            userName: 'Bob',
            groupId: 'g_sales',
            groupNames: ['Sales'],
        }),
        row({ id: 'b2', userId: 'bob', userName: 'Bob', groupId: 'g_ops', groupNames: ['Ops'] }),
    ]
    const folded = foldMembers(rows)
    expect(folded.map(r => [r.id, r.role, r.groupId, r.groupNames])).toEqual([
        ['a2', 'owner', null, []],
        ['b1', 'member', 'g_sales', ['Sales', 'Ops']],
    ])
})

test('foldMembers: a derived row with no group name never wins over the direct row', () => {
    const folded = foldMembers([
        row({ id: 'a1', userId: 'ann', groupId: 'g_hidden' }),
        row({ id: 'a2', userId: 'ann' }),
    ])
    expect(folded.map(r => [r.id, r.groupId])).toEqual([['a2', null]])
})

test('viaGroupLabel names one group, several groups, or an unknown group', () => {
    expect(viaGroupLabel(['Sales'])).toBe('Through group Sales')
    expect(viaGroupLabel(['Sales', 'Ops'])).toBe('Through groups Sales, Ops')
    expect(viaGroupLabel([])).toBe('Through a group')
})
