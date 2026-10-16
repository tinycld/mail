// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

const h = vi.hoisted(() => ({
    identities: [{ id: 'i1', default_from: '' }],
    updated: [] as Record<string, unknown>[],
}))

afterEach(() => {
    cleanup()
    h.updated.length = 0
})

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { BasicIndex, createCollection, localOnlyCollectionOptions } = await import(
        '@tanstack/db'
    )
    const identities = createCollection({
        ...localOnlyCollectionOptions({
            id: 'mail_identities',
            getKey: (r: { id: string }) => r.id,
            initialData: h.identities,
            onUpdate: async ({ transaction }) => {
                for (const m of transaction.mutations) h.updated.push(m.modified)
            },
        }),
        autoIndex: 'eager',
        defaultIndexType: BasicIndex,
    })
    return { useStore: (...names: string[]) => names.map(() => identities) }
})

import { DefaultFromPicker } from '~/tinycld/mail/settings/DefaultFromPicker'

const addresses = [
    { id: 'a1', localPart: 'nathan', domainName: 'tinycld.com', isPrimary: true, rank: 1 },
    { id: 'a2', localPart: 'nathan', domainName: 'q.example', isPrimary: false, rank: 2 },
].map(a => ({ ...a, domainId: 'd', source: 'rule', groupName: null }))

function renderPicker(value: string) {
    return render(
        <QueryClientProvider client={new QueryClient()}>
            <DefaultFromPicker identityId="i1" addresses={addresses} value={value} />
        </QueryClientProvider>
    )
}

async function press(el: HTMLElement) {
    await act(async () => {
        fireEvent.click(el)
    })
}

it('choosing an address writes default_from', async () => {
    const { findByText } = renderPicker('')
    await press(await findByText('nathan@q.example'))
    await waitFor(() => expect(h.updated).toHaveLength(1))
    expect(h.updated[0]).toMatchObject({ id: 'i1', default_from: 'a2' })
})

it("choosing the main domain's address writes an empty default_from", async () => {
    const { findByText } = renderPicker('a2')
    await press(await findByText("The main domain's address"))
    await waitFor(() => expect(h.updated).toHaveLength(1))
    expect(h.updated[0]).toMatchObject({ id: 'i1', default_from: '' })
})
