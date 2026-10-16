// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(cleanup)

// The group name comes from a left join: this mounts the real query so an
// address no group gave proves the join yields null, not a dropped row.
const h = vi.hoisted(() => ({
    domains: [
        { id: 'd_q', domain: 'q.example', coverage: 'groups', rank: 1 },
        { id: 'd_com', domain: 'tinycld.com', coverage: 'everyone', rank: 2 },
    ],
    groups: [{ id: 'g_sales', name: 'Sales' }],
    addresses: [
        {
            id: 'a_grp',
            local_part: 'ann',
            domain: 'd_q',
            mailbox: 'mb1',
            source: 'rule',
            is_primary: false,
            via_group: 'g_sales',
        },
        {
            id: 'a_rule',
            local_part: 'ann',
            domain: 'd_com',
            mailbox: 'mb1',
            source: 'rule',
            is_primary: false,
            via_group: '',
        },
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
        mail_domains: mk('mail_domains', h.domains),
        mail_addresses: mk('mail_addresses', h.addresses),
        groups: mk('groups', h.groups),
    }
    return { useStore: (...names: string[]) => names.map(n => stores[n]) }
})

import { MailboxAddressesPanel } from '~/tinycld/mail/settings/MailboxAddressesPanel'

test('a rule address names the group that gave it; a domain rule address does not', async () => {
    const { findByText } = render(
        <QueryClientProvider client={new QueryClient()}>
            <MailboxAddressesPanel mailboxId="mb1" />
        </QueryClientProvider>
    )
    expect(await findByText('From group Sales')).toBeTruthy()
    expect(await findByText('From the domain rule')).toBeTruthy()
    expect(await findByText('ann@tinycld.com')).toBeTruthy()
})
