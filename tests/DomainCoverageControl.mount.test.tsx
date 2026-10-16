// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(cleanup)

const h = vi.hoisted(() => ({
    domains: [{ id: 'd_com', domain: 'tinycld.com', coverage: 'manual', rank: 1 }],
    users: [
        { id: 'u1', role: 'member' },
        { id: 'u2', role: 'guest' },
        { id: 'u3', role: 'member' },
    ],
    identities: [
        { id: 'i1', user: 'u1', email_name: 'nathan', inbox: 'mb1', retired: false },
        { id: 'i2', user: 'u2', email_name: 'gus', inbox: 'mb2', retired: false },
        { id: 'i3', user: 'u3', email_name: 'retired-i3', inbox: '', retired: true },
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
    }
    return { useStore: (...names: string[]) => names.map(n => stores[n]) }
})

import { DomainCoverageControl } from '~/tinycld/mail/settings/DomainCoverageControl'

const withQueryClient = (ui: ReactNode) => (
    <QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>
)

function renderControl() {
    return render(
        withQueryClient(
            <DomainCoverageControl domainId="d_com" domainName="tinycld.com" coverage="manual" />
        )
    )
}

async function press(el: HTMLElement) {
    await act(async () => {
        fireEvent.click(el)
    })
}

test('choosing Everyone previews only active non-guest people', async () => {
    const { findByText } = renderControl()
    await press(await findByText('Everyone'))
    expect(await findByText('Adds 1: nathan@tinycld.com')).toBeTruthy()
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
    expect(queryByText('Adds 1: nathan@tinycld.com')).toBeNull()
    // The test renderer stringifies accessibilityState; the selected dot is
    // the observable difference.
    const manual = container.querySelector('[testid="coverage-manual"]')
    const everyone = container.querySelector('[testid="coverage-everyone"]')
    expect(manual?.innerHTML).toContain('border-primary')
    expect(everyone?.innerHTML).not.toContain('border-primary')
})
