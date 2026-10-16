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
import { foldMembers, type MemberRow } from '~/tinycld/mail/settings/member-rows'

function row(over: Partial<MemberRow>): MemberRow {
    return {
        id: 'r',
        userId: 'u',
        userName: 'U',
        userEmail: '',
        role: 'member',
        isYou: false,
        viaGroupName: null,
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
    viaGroupName: 'Sales',
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

test('foldMembers: one row per person, the direct row wins, group-only rows name their groups', () => {
    const rows = [
        row({ id: 'a1', userId: 'ann', userName: 'Ann', viaGroupName: 'Sales' }),
        row({ id: 'a2', userId: 'ann', userName: 'Ann', viaGroupName: null, role: 'owner' }),
        row({ id: 'b1', userId: 'bob', userName: 'Bob', viaGroupName: 'Sales' }),
        row({ id: 'b2', userId: 'bob', userName: 'Bob', viaGroupName: 'Ops' }),
    ]
    const folded = foldMembers(rows)
    expect(folded.map(r => [r.id, r.role, r.viaGroupName])).toEqual([
        ['a2', 'owner', null],
        ['b1', 'member', 'Sales, Ops'],
    ])
})
