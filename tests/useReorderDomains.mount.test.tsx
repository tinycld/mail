// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(cleanup)

const h = vi.hoisted(() => ({
    domains: [
        { id: 'd1', domain: 'a.com', rank: 1 },
        { id: 'd2', domain: 'b.com', rank: 2 },
        { id: 'd3', domain: 'c.com', rank: 3 },
    ],
    updated: [] as Record<string, unknown>[],
}))

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { createCollection, localOnlyCollectionOptions } = await import('@tanstack/db')
    const domains = createCollection(
        localOnlyCollectionOptions({
            id: 'mail_domains',
            getKey: (r: { id: string }) => r.id,
            initialData: h.domains,
            onUpdate: async ({ transaction }) => {
                for (const m of transaction.mutations) h.updated.push(m.modified)
            },
        })
    )
    return { useStore: () => [domains] }
})

import { useReorderDomains } from '~/tinycld/mail/settings/useReorderDomains'

const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>
)

test('the dragged order becomes ranks 1..n', async () => {
    const { result } = renderHook(() => useReorderDomains(), { wrapper })
    await act(async () => {
        result.current.mutate(['d3', 'd1', 'd2'])
    })
    await waitFor(() => expect(h.updated).toHaveLength(3))
    const ranks = Object.fromEntries(h.updated.map(r => [r.id, r.rank]))
    expect(ranks).toEqual({ d3: 1, d1: 2, d2: 3 })
})
