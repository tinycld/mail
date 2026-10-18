// @vitest-environment happy-dom
import type { Collection } from '@tanstack/db'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(() => {
    cleanup()
    h.onNavigate.mockClear()
})

const h = vi.hoisted(() => ({
    userId: 'u1',
    onNavigate: vi.fn(),
    stores: {} as Record<string, unknown>,
    identities: [
        {
            id: 'i1',
            user: 'u1',
            inbox: 'mb1',
            retired: false,
            address_views: true,
            hidden_address_views: [] as string[],
        },
        {
            id: 'i2',
            user: 'u2',
            inbox: 'mb2',
            retired: false,
            address_views: false,
            hidden_address_views: [] as string[],
        },
    ],
    addresses: [
        { id: 'a1', local_part: 'nathan', domain: 'd1', mailbox: 'mb1' },
        { id: 'a2', local_part: 'nathan', domain: 'd2', mailbox: 'mb1' },
        { id: 'a3', local_part: 'ann', domain: 'd1', mailbox: 'mb2' },
        { id: 'a4', local_part: 'ann', domain: 'd2', mailbox: 'mb2' },
    ],
    domains: [
        { id: 'd1', domain: 'tinycld.com', rank: 1 },
        { id: 'd2', domain: 'q.example', rank: 2 },
    ],
    counts: [
        {
            id: 'u1mb1',
            user: 'u1',
            mailbox: 'mb1',
            inbox: 4,
            inbox_total: 9,
            inbox_by_address: { a1: { unread: 3, total: 5 }, a2: { unread: 0, total: 4 } },
        },
        {
            id: 'u2mb2',
            user: 'u2',
            mailbox: 'mb2',
            inbox: 1,
            inbox_total: 1,
            inbox_by_address: { a3: { unread: 1, total: 1 } },
        },
    ],
}))

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: { id: h.userId }, isLoggedIn: true }),
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
    h.stores.mail_identities = mk('mail_identities', h.identities)
    h.stores.mail_addresses = mk('mail_addresses', h.addresses)
    h.stores.mail_domains = mk('mail_domains', h.domains)
    h.stores.mail_folder_counts = mk('mail_folder_counts', h.counts)
    return { useStore: (...names: string[]) => names.map(n => h.stores[n]) }
})

import { MailboxSidebarSection } from '~/tinycld/mail/components/MailboxSidebarSection'
import { EMPTY_COUNTS } from '~/tinycld/mail/hooks/useMailboxFolderCounts'

function renderSection(userId: string, mailboxId: string) {
    h.userId = userId
    return render(
        <QueryClientProvider client={new QueryClient()}>
            <MailboxSidebarSection
                mailboxId={mailboxId}
                mailboxLabel="Personal"
                isPersonal
                defaultExpanded
                counts={EMPTY_COUNTS}
                activeMailboxId={mailboxId}
                activeFolder="inbox"
                onNavigate={h.onNavigate}
            />
        </QueryClientProvider>
    )
}

// react-native-web under happy-dom renders testID as a plain `testid` attribute.
function byTestId(container: HTMLElement, testId: string) {
    return container.querySelector<HTMLElement>(`[testid="${testId}"]`)
}

test('with two views, entries render under the personal Inbox with their unread badges', async () => {
    const { container, getByText } = renderSection('u1', 'mb1')
    await waitFor(() => expect(byTestId(container, 'address-view-nathan@tinycld.com')).toBeTruthy())
    const first = byTestId(container, 'address-view-nathan@tinycld.com')
    const second = byTestId(container, 'address-view-nathan@q.example')
    expect(first?.textContent).toBe('nathan@tinycld.com3')
    expect(second?.textContent).toBe('nathan@q.example')

    // Under the Inbox item: the Inbox comes first, Starred after the views.
    const text = container.textContent ?? ''
    expect(text.indexOf('Inbox')).toBeLessThan(text.indexOf('nathan@tinycld.com'))
    expect(text.indexOf('nathan@q.example')).toBeLessThan(text.indexOf('Starred'))
    expect(getByText('Inbox')).toBeTruthy()
})

test('pressing a view navigates to its address folder', async () => {
    const { container } = renderSection('u1', 'mb1')
    await waitFor(() => expect(byTestId(container, 'address-view-nathan@q.example')).toBeTruthy())
    const entry = byTestId(container, 'address-view-nathan@q.example')
    if (!entry) throw new Error('no entry')
    await act(async () => {
        fireEvent.click(entry)
    })
    expect(h.onNavigate).toHaveBeenCalledWith('mb1', 'address:a2')
})

test('with the switch off, no entries render', async () => {
    const { container } = renderSection('u2', 'mb2')
    await waitFor(() => expect(byTestId(container, 'address-views-ready')).toBeTruthy())
    expect(container.querySelector('[testid^="address-view-"]')).toBeNull()

    // The same person with the switch on gets the entries: the absence above
    // came from the switch, not from queries that had not loaded yet.
    const identities = h.stores.mail_identities as Collection<{
        id: string
        address_views: boolean
    }>
    await act(async () => {
        identities.update('i2', d => {
            d.address_views = true
        })
    })
    await waitFor(() => expect(byTestId(container, 'address-view-ann@tinycld.com')).toBeTruthy())
    expect(byTestId(container, 'address-view-ann@tinycld.com')?.textContent).toBe(
        'ann@tinycld.com1'
    )
})

test('the ready marker waits for the views query to load', async () => {
    const { createCollection } = await import('@tanstack/db')
    const loaded = h.stores.mail_identities
    // A collection whose sync never marks it ready: the query stays loading.
    h.stores.mail_identities = createCollection({
        id: 'mail_identities_loading',
        getKey: (r: { id: string }) => r.id,
        sync: { sync: () => {} },
    })
    try {
        const { container } = renderSection('u1', 'mb1')
        await waitFor(() => expect(byTestId(container, 'mailbox-section')).toBeTruthy())
        expect(byTestId(container, 'address-views-ready')).toBeNull()
    } finally {
        h.stores.mail_identities = loaded
    }
})
