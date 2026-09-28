// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, expect, test, vi } from 'vitest'

// No vitest globals in this workspace, so testing-library's automatic
// afterEach cleanup never registers.
afterEach(cleanup)
afterEach(() => {
    seenQueries.length = 0
})

// mail_mailbox_aliases' list rule is org-wide non-guest, so an unscoped query
// here would pull every alias in the deployment to validate one new one. This
// mounts the real component against real TanStack DB collections so the
// compiled query is what a test actually inspects — a fixture with only one
// domain's worth of data would pass even if the `where` clause were dropped.
const h = vi.hoisted(() => ({
    mailboxes: [
        { id: 'mb_d1_a', address: 'alice', domain: 'd1', created: '', updated: '' },
        { id: 'mb_d1_b', address: 'support', domain: 'd1', created: '', updated: '' },
        // A different domain's mailbox — must never appear in the scoped query.
        { id: 'mb_d2_a', address: 'bob', domain: 'd2', created: '', updated: '' },
    ],
    aliases: [
        { id: 'a1', mailbox: 'mb_d1_b', address: 'help', created: '', updated: '' },
        // Another domain's alias — must never be pulled down by this form.
        { id: 'a2', mailbox: 'mb_d2_a', address: 'sales', created: '', updated: '' },
    ],
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
        mail_mailboxes: mk('mail_mailboxes', h.mailboxes),
        mail_mailbox_aliases: mk('mail_mailbox_aliases', h.aliases),
    }
    return {
        useStore: (...names: string[]) => names.map(n => stores[n]),
    }
})

// Capture each compiled query so the test can assert on the predicate shape,
// not just the result — the bounding lives in the query.
const seenQueries: string[] = []

const serializeWhere = (where: unknown): string => {
    const seen = new WeakSet()
    return JSON.stringify(where, (key, value) => {
        if (key === 'collection') return undefined
        if (typeof value === 'object' && value !== null) {
            if (seen.has(value)) return undefined
            seen.add(value)
        }
        return value
    })
}

type QueryFn = (q: never) => { query?: { from?: unknown; where?: unknown } } | null | undefined

const recordQuery = (fn: QueryFn) => (q: never) => {
    const built = fn(q)
    if (built?.query) {
        seenQueries.push(`${serializeWhere(built.query.from)}|${serializeWhere(built.query.where)}`)
    }
    return built as never
}

vi.mock('@tanstack/react-db', async () => {
    const actual = await vi.importActual<typeof import('@tanstack/react-db')>('@tanstack/react-db')
    return {
        ...actual,
        useLiveQuery: (arg: QueryFn | { query: QueryFn }, deps?: unknown[]) => {
            const config = typeof arg === 'function' ? { query: arg } : arg
            return actual.useLiveQuery(
                { ...config, query: recordQuery(config.query) } as never,
                deps as never
            )
        },
    }
})

import { AddAliasForm } from '~/tinycld/mail/settings/AddAliasForm'

const withQueryClient = (ui: ReactNode) => (
    <QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>
)

test('the alias-collision query is scoped to the mailbox domain, not every alias in the org', async () => {
    render(
        withQueryClient(
            <AddAliasForm mailboxId="mb_d1_b" mailboxDomainId="d1" domainName="acme.com" />
        )
    )

    await waitFor(() => expect(seenQueries.length).toBeGreaterThan(0))

    const aliasQuery = seenQueries.find(q => q.includes('mail_mailbox_aliases'))
    expect(aliasQuery).toBeDefined()
    // Scoped to this domain's mailbox ids via inArray...
    expect(aliasQuery).toContain('mb_d1_a')
    expect(aliasQuery).toContain('mb_d1_b')
    // ...and never names the other domain's mailbox, so it cannot pull its alias.
    expect(aliasQuery).not.toContain('mb_d2_a')
})
