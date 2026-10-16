// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(cleanup)

const h = vi.hoisted(() => ({
    domains: [{ id: 'd1', domain: 'tinycld.com', rank: 1 }],
    mailboxes: [
        {
            id: 'mb1',
            type: 'personal',
            address: 'nathan',
            domain: 'd1',
            display_name: 'Nathan',
            name: '',
            created: '',
        },
    ],
}))

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: { id: 'u1' }, isLoggedIn: true }),
}))

vi.mock('@tinycld/core/lib/use-current-role', () => ({
    useCurrentRole: () => ({ isAdmin: true }),
}))

vi.mock('@tinycld/core/lib/org-routes', () => ({
    useOrgHref: () => () => '/settings/mail/provider',
}))

vi.mock('@tinycld/core/components/help/HelpIcon', () => ({ HelpIcon: () => null }))

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
        mail_domains: mk('mail_domains', h.domains),
        mail_mailboxes: mk('mail_mailboxes', h.mailboxes),
        mail_mailbox_members: mk('mail_mailbox_members', []),
        mail_addresses: mk('mail_addresses', []),
        users: mk('users', []),
        groups: mk('groups', []),
    }
    return { useStore: (...names: string[]) => names.map(n => stores[n]) }
})

import MailboxesSettings from '~/tinycld/mail/settings/mailboxes'

function renderScreen() {
    return render(
        <QueryClientProvider client={new QueryClient()}>
            <MailboxesSettings />
        </QueryClientProvider>
    )
}

test('a personal inbox alone leaves the shared list empty', async () => {
    const { findByText, queryByText } = renderScreen()
    expect(await findByText('No shared inboxes yet')).toBeTruthy()
    expect(queryByText('nathan')).toBeNull()
})

test('the create button says New shared inbox', async () => {
    const { findByText } = renderScreen()
    expect(await findByText('New shared inbox')).toBeTruthy()
})
