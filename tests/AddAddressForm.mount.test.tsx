// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

// No vitest globals in this workspace, so testing-library's automatic
// afterEach cleanup never registers.
afterEach(cleanup)

// Mounts the real form against real TanStack DB collections so the rules the
// schema is built from (taken addresses, reserved email names, rule domains)
// come out of executed queries, not fixtures handed to the schema.
const h = vi.hoisted(() => ({
    domains: [
        { id: 'd_org', domain: 'tinycld.org', coverage: 'manual', rank: 1 },
        { id: 'd_com', domain: 'tinycld.com', coverage: 'everyone', rank: 2 },
        { id: 'd_grp', domain: 'q.example', coverage: 'groups', rank: 3 },
    ],
    addresses: [{ id: 'a1', local_part: 'help', domain: 'd_org', mailbox: 'mb_other' }],
    identities: [{ id: 'i1', email_name: 'holly', inbox: 'mb_other', user: 'u_holly' }],
    inserted: [] as Record<string, unknown>[],
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
            }),
            autoIndex: 'eager',
            defaultIndexType: BasicIndex,
        })
    const stores: Record<string, unknown> = {
        mail_domains: mk('mail_domains', h.domains),
        mail_addresses: mk('mail_addresses', h.addresses),
        mail_identities: mk('mail_identities', h.identities),
    }
    return {
        useStore: (...names: string[]) => names.map(n => stores[n]),
    }
})

import { AddAddressForm } from '~/tinycld/mail/settings/AddAddressForm'

const withQueryClient = (ui: ReactNode) => (
    <QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>
)

async function typeAddress(container: HTMLElement, value: string) {
    const input = container.querySelector<HTMLInputElement>('rn-textinput')
    if (!input) throw new Error('no input rendered')
    // mode: 'onChange' validates asynchronously; let it settle.
    await act(async () => {
        fireEvent.input(input, { target: { value } })
    })
}

async function chooseDomain(findByText: (t: string) => Promise<HTMLElement>, name: string) {
    const option = await findByText(name)
    await act(async () => {
        fireEvent.click(option)
    })
}

function renderForm() {
    return render(withQueryClient(<AddAddressForm mailboxId="mb1" onDone={vi.fn()} />))
}

const RESERVED = "This is a person's email name, and this domain's rule keeps that name for them"

test("a person's email name is refused on a domain with an Everyone rule", async () => {
    const { container, findByText } = renderForm()
    await chooseDomain(findByText, 'tinycld.com')
    await typeAddress(container, 'holly')
    expect(await findByText(RESERVED)).toBeTruthy()
})

// The server reserves email names on a groups domain too, for members and
// non-members alike (server/addresses.go checkAddressAvailable).
test("a person's email name is refused on a domain with a groups rule", async () => {
    const { container, findByText } = renderForm()
    await chooseDomain(findByText, 'q.example')
    await typeAddress(container, 'holly')
    expect(await findByText(RESERVED)).toBeTruthy()
})

test('an address that already exists on the domain is refused', async () => {
    const { container, findByText } = renderForm()
    await chooseDomain(findByText, 'tinycld.org')
    await typeAddress(container, 'help')
    expect(await findByText('This address already exists')).toBeTruthy()
})

test('a free address on a manual domain is added as a hand-made, non-main address', async () => {
    const { container, findByText } = renderForm()
    await chooseDomain(findByText, 'tinycld.org')
    await typeAddress(container, 'sales')
    const button = await findByText('Add address')
    await waitFor(() => expect(container.querySelector('[aria-disabled="true"]')).toBeNull())
    await act(async () => {
        fireEvent.click(button)
    })
    await waitFor(() => expect(h.inserted).toHaveLength(1))
    expect(h.inserted[0]).toMatchObject({
        local_part: 'sales',
        domain: 'd_org',
        mailbox: 'mb1',
        source: 'manual',
        is_primary: false,
    })
})
