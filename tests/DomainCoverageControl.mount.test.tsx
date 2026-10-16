// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(() => {
    cleanup()
    h.updated.length = 0
})

const h = vi.hoisted(() => ({
    domains: [
        { id: 'd_com', domain: 'tinycld.com', coverage: 'manual', rank: 1 },
        { id: 'd_q', domain: 'q.example', coverage: 'manual', rank: 2 },
    ],
    users: [
        { id: 'u1', role: 'member' },
        { id: 'u2', role: 'guest' },
        { id: 'u3', role: 'member' },
        { id: 'u4', role: 'member' },
        { id: 'u5', role: 'member' },
        { id: 'u6', role: 'member' },
    ],
    identities: [
        { id: 'i1', user: 'u1', email_name: 'nathan', inbox: 'mb1', retired: false },
        { id: 'i2', user: 'u2', email_name: 'gus', inbox: 'mb2', retired: false },
        { id: 'i3', user: 'u3', email_name: 'retired-i3', inbox: '', retired: true },
        // Not retired, but the inbox is gone (deleted outside offboarding):
        // the server gives it no address until the identity is repaired.
        { id: 'i4', user: 'u4', email_name: 'orphan', inbox: '', retired: false },
        { id: 'i5', user: 'u5', email_name: 'ann', inbox: 'mb5', retired: false },
        { id: 'i6', user: 'u6', email_name: 'bob', inbox: 'mb6', retired: false },
    ],
    groups: [
        { id: 'g_sales', name: 'Sales' },
        { id: 'g_ops', name: 'Ops' },
    ],
    groupMembers: [
        { id: 'gm1', group: 'g_sales', user: 'u5' },
        { id: 'gm2', group: 'g_ops', user: 'u6' },
        // A guest is never given a rule address, also as a group member.
        { id: 'gm3', group: 'g_sales', user: 'u2' },
    ],
    addresses: [] as { id: string }[],
    updated: [] as Record<string, unknown>[],
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
        mail_domains: mk('mail_domains', h.domains),
        mail_identities: mk('mail_identities', h.identities),
        users: mk('users', h.users),
        mail_addresses: mk('mail_addresses', h.addresses),
        groups: mk('groups', h.groups),
        group_members: mk('group_members', h.groupMembers),
    }
    return { useStore: (...names: string[]) => names.map(n => stores[n]) }
})

import { DomainCoverageControl } from '~/tinycld/mail/settings/DomainCoverageControl'

const withQueryClient = (ui: ReactNode) => (
    <QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>
)

function control(coverage: 'everyone' | 'manual') {
    return withQueryClient(
        <DomainCoverageControl
            domainId="d_com"
            domainName="tinycld.com"
            coverage={coverage}
            coverageGroups={[]}
        />
    )
}

function renderGroupsDomain() {
    return render(
        withQueryClient(
            <DomainCoverageControl
                domainId="d_q"
                domainName="q.example"
                coverage="manual"
                coverageGroups={[]}
            />
        )
    )
}

function renderControl() {
    return render(control('manual'))
}

async function press(el: HTMLElement) {
    await act(async () => {
        fireEvent.click(el)
    })
}

test('choosing Everyone previews only active non-guest people with an inbox', async () => {
    const { findByText } = renderControl()
    await press(await findByText('Everyone'))
    expect(
        await findByText('Adds 3: ann@tinycld.com, bob@tinycld.com, nathan@tinycld.com')
    ).toBeTruthy()
})

test('Apply writes the coverage to the domain row', async () => {
    const { findByText } = renderControl()
    await press(await findByText('Everyone'))
    await press(await findByText('Apply'))
    await waitFor(() => expect(h.updated).toHaveLength(1))
    expect(h.updated[0]).toMatchObject({ id: 'd_com', coverage: 'everyone' })
})

test('Cancel removes the confirm box', async () => {
    const { container, findByText, queryByText } = renderControl()
    await press(await findByText('Everyone'))
    await press(await findByText('Cancel'))
    await waitFor(() => expect(queryByText('Apply')).toBeNull())
    expect(queryByText(/^Adds/)).toBeNull()
    // The test renderer stringifies accessibilityState; the selected dot is
    // the observable difference.
    const manual = container.querySelector('[testid="coverage-manual"]')
    const everyone = container.querySelector('[testid="coverage-everyone"]')
    expect(manual?.innerHTML).toContain('border-primary')
    expect(everyone?.innerHTML).not.toContain('border-primary')
})

// Another admin, or the Apply of this one, changes the domain row: the form
// must show the saved rule, not offer to change it back.
test('a new coverage from the domain row resets the form', async () => {
    const { container, rerender, queryByText } = renderControl()
    await act(async () => {
        rerender(control('everyone'))
    })
    await waitFor(() => {
        const everyone = container.querySelector('[testid="coverage-everyone"]')
        expect(everyone?.innerHTML).toContain('border-primary')
    })
    expect(queryByText('Apply')).toBeNull()
})

test('the group picker shows only for Members of these groups', async () => {
    const { findByText, queryByText } = renderGroupsDomain()
    await findByText('Members of these groups')
    expect(queryByText('Sales')).toBeNull()
    await press(await findByText('Members of these groups'))
    expect(await findByText('Sales')).toBeTruthy()
    expect(await findByText('Ops')).toBeTruthy()
})

test('Members of these groups with no group: Apply shows the error and writes nothing', async () => {
    const { findByText, findAllByText } = renderGroupsDomain()
    await press(await findByText('Members of these groups'))
    await press(await findByText('Apply'))
    expect((await findAllByText(/Choose at least one group/)).length).toBeGreaterThan(0)
    expect(h.updated).toHaveLength(0)
})

test('choosing Sales previews only its live members', async () => {
    const { findByText, queryByText } = renderGroupsDomain()
    await press(await findByText('Members of these groups'))
    await press(await findByText('Sales'))
    expect(await findByText('Adds 1: ann@q.example')).toBeTruthy()
    expect(queryByText(/bob@q\.example/)).toBeNull()
})

test('Apply writes the groups coverage and the chosen groups', async () => {
    const { findByText } = renderGroupsDomain()
    await press(await findByText('Members of these groups'))
    await press(await findByText('Sales'))
    await findByText('Adds 1: ann@q.example')
    await press(await findByText('Apply'))
    await waitFor(() => expect(h.updated).toHaveLength(1))
    expect(h.updated[0]).toMatchObject({
        id: 'd_q',
        coverage: 'groups',
        coverage_groups: ['g_sales'],
    })
})

// A pick left behind in the hidden picker is not a change, and is not saved.
test('a group pick left behind when switching back is not a change', async () => {
    const { findByText, queryByText } = renderGroupsDomain()
    await press(await findByText('Members of these groups'))
    await press(await findByText('Sales'))
    await findByText('Adds 1: ann@q.example')
    await press(await findByText('Only people I add'))
    await waitFor(() => expect(queryByText('Apply')).toBeNull())
})

test('switching to another rule keeps the stored groups', async () => {
    const { findByText } = render(
        withQueryClient(
            <DomainCoverageControl
                domainId="d_q"
                domainName="q.example"
                coverage="everyone"
                coverageGroups={['g_ops']}
            />
        )
    )
    await press(await findByText('Members of these groups'))
    await press(await findByText('Sales'))
    await press(await findByText('Only people I add'))
    await press(await findByText('Apply'))
    await waitFor(() => expect(h.updated).toHaveLength(1))
    expect(h.updated[0]).toMatchObject({
        id: 'd_q',
        coverage: 'manual',
        coverage_groups: ['g_ops'],
    })
})

function renderSavedGroupsRule(coverageGroups: string[]) {
    return render(
        withQueryClient(
            <DomainCoverageControl
                domainId="d_q"
                domainName="q.example"
                coverage="groups"
                coverageGroups={coverageGroups}
            />
        )
    )
}

// Deleting a domain's last group leaves a groups rule with no group: the
// card must say that nobody gets an address here.
test('a saved groups rule with no group shows a warning', async () => {
    const { findByText } = renderSavedGroupsRule([])
    expect(await findByText(/Nobody gets an address on q\.example/)).toBeTruthy()
})

test('a saved groups rule with a group shows no warning', async () => {
    const { findByText, queryByText } = renderSavedGroupsRule(['g_sales'])
    await findByText('Members of these groups')
    expect(queryByText(/Nobody gets an address/)).toBeNull()
})
