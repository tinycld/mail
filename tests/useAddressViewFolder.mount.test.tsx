// @vitest-environment happy-dom
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(cleanup)

// The list screen disables its thread query (no mailbox) when this hook calls
// the folder hidden, so `isHidden` is what makes a stale `folder=address:<id>`
// link list nothing, as the IMAP folder does.

const h = vi.hoisted(() => ({
    userId: 'u1',
    identities: [
        {
            id: 'i1',
            user: 'u1',
            inbox: 'mb1',
            retired: false,
            address_views: true,
            hidden_address_views: ['a3'],
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
        { id: 'a3', local_part: 'sales', domain: 'd1', mailbox: 'mb1' },
        { id: 'a4', local_part: 'ann', domain: 'd1', mailbox: 'mb2' },
        { id: 'a5', local_part: 'ann', domain: 'd2', mailbox: 'mb2' },
    ],
    domains: [
        { id: 'd1', domain: 'tinycld.com', rank: 1 },
        { id: 'd2', domain: 'q.example', rank: 2 },
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
    const stores: Record<string, unknown> = {
        mail_identities: mk('mail_identities', h.identities),
        mail_addresses: mk('mail_addresses', h.addresses),
        mail_domains: mk('mail_domains', h.domains),
        mail_folder_counts: mk('mail_folder_counts', []),
    }
    return { useStore: (...names: string[]) => names.map(n => stores[n]) }
})

import { useAddressViewFolder } from '~/tinycld/mail/hooks/useAddressViews'

function renderFolder(userId: string, folder: string | null) {
    h.userId = userId
    return renderHook(() => useAddressViewFolder(folder))
}

test('a shown view is titled by its address and lists its threads', async () => {
    const { result } = renderFolder('u1', 'address:a2')
    await waitFor(() => expect(result.current.title).toBe('nathan@q.example'))
    expect(result.current.isHidden).toBe(false)
})

test('a hidden address is a view that is not shown: it lists nothing', async () => {
    const { result } = renderFolder('u1', 'address:a3')
    await waitFor(() => expect(result.current.isHidden).toBe(true))
    expect(result.current.title).toBe('Address not shown')
})

test('an id that is no address of the inbox lists nothing', async () => {
    const { result } = renderFolder('u1', 'address:gone')
    await waitFor(() => expect(result.current.isHidden).toBe(true))
})

test('with the switch off, a view link lists nothing', async () => {
    const { result } = renderFolder('u2', 'address:a4')
    await waitFor(() => expect(result.current.isHidden).toBe(true))
    expect(result.current.title).toBe('Address not shown')
})

test('any other folder is not an address view', () => {
    const { result } = renderFolder('u1', 'inbox')
    expect(result.current).toEqual({ isHidden: false, title: null })
})
