// @vitest-environment happy-dom
import { renderHook, waitFor } from '@testing-library/react'
import { expect, it, vi } from 'vitest'

const h = vi.hoisted(() => ({
    identities: [
        {
            id: 'i1',
            user: 'u1',
            email_name: 'nathan',
            inbox: 'mb1',
            default_from: '',
            retired: false,
        },
        { id: 'i2', user: 'u2', email_name: 'ann', inbox: 'mb2', default_from: '', retired: false },
    ],
    addresses: [
        {
            id: 'a1',
            local_part: 'nathan',
            domain: 'd1',
            mailbox: 'mb1',
            source: 'rule',
            is_primary: true,
            via_group: '',
        },
        {
            id: 'a2',
            local_part: 'nathan',
            domain: 'd2',
            mailbox: 'mb1',
            source: 'manual',
            is_primary: false,
            via_group: '',
        },
    ],
    domains: [
        { id: 'd1', domain: 'tinycld.com', rank: 1 },
        { id: 'd2', domain: 'q.example', rank: 2 },
    ],
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
            }),
            autoIndex: 'eager',
            defaultIndexType: BasicIndex,
        })
    const stores: Record<string, unknown> = {
        mail_identities: mk('mail_identities', h.identities),
        mail_addresses: mk('mail_addresses', h.addresses),
        mail_domains: mk('mail_domains', h.domains),
        groups: mk('groups', []),
    }
    return { useStore: (...names: string[]) => names.map(n => stores[n]) }
})

import { usePersonEmail } from '~/tinycld/mail/settings/usePersonEmail'

it('returns the identity and both addresses', async () => {
    const { result } = renderHook(() => usePersonEmail('u1'))
    await waitFor(() => expect(result.current.addresses).toHaveLength(2))
    expect(result.current.identity).toMatchObject({ id: 'i1', emailName: 'nathan', inbox: 'mb1' })
    expect(result.current.addresses.map(a => a.id).sort()).toEqual(['a1', 'a2'])
})

it('returns the identity with no addresses for a person without a covered domain', async () => {
    const { result } = renderHook(() => usePersonEmail('u2'))
    await waitFor(() => expect(result.current.identity).toMatchObject({ id: 'i2' }))
    expect(result.current.addresses).toEqual([])
})

it('returns null for a user with no identity', async () => {
    const { result } = renderHook(() => usePersonEmail('u9'))
    await waitFor(() => expect(result.current.isReady).toBe(true))
    expect(result.current.identity).toBeNull()
    expect(result.current.addresses).toEqual([])
})
