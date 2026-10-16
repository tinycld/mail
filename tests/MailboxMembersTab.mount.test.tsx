// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, waitFor, within } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(cleanup)

vi.mock('@tinycld/core/lib/use-app-theme', () => ({ useThemeColor: () => '#000000' }))
vi.mock('@tinycld/core/lib/use-writes-available', () => ({
    useServerActionState: ({ isDisabled }: { isDisabled: boolean }) => ({
        isDisabled,
        accessibilityHint: '',
    }),
}))

// The shared stub lists icons by name and has no UsersRound, which the core
// group section draws.
vi.mock('lucide-react-native', async importOriginal => ({
    ...(await importOriginal<Record<string, unknown>>()),
    UsersRound: () => null,
}))

// The real drawer portals into a native modal host that happy-dom does not
// render; the members tab under test does not depend on it.
vi.mock('@tinycld/core/ui/drawer', () => {
    const Passthrough = ({ children }: { children?: React.ReactNode }) => <>{children}</>
    return {
        Drawer: Passthrough,
        DrawerBackdrop: () => null,
        DrawerBody: Passthrough,
        DrawerCloseButton: Passthrough,
        DrawerContent: Passthrough,
        DrawerFooter: Passthrough,
        DrawerHeader: Passthrough,
    }
})

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
import { MailboxDrawer } from '~/tinycld/mail/settings/MailboxDrawer'
import { foldMembers, type MemberRow } from '~/tinycld/mail/settings/member-rows'

const mailbox = {
    id: 'mb1',
    address: 'support',
    domain: 'd1',
    domainName: 'example.com',
    displayName: 'Support',
    type: 'shared' as const,
    createdAt: '',
}

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

function mountDrawer(members: MemberRow[]) {
    return render(
        <QueryClientProvider client={new QueryClient()}>
            <OverlayProvider>
                <MailboxDrawer
                    mode={{ kind: 'view', mailboxId: 'mb1' }}
                    onClose={() => {}}
                    onSwitchToEdit={() => {}}
                    onSwitchToView={() => {}}
                    mailbox={mailbox}
                    members={members}
                    orgMembers={[]}
                    domainOptions={[]}
                    userId="me"
                />
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

test('the tab title counts distinct people', async () => {
    const { findByText } = mountDrawer([me, derived])
    expect(await findByText('Members · 2')).toBeTruthy()
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
