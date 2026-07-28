// @vitest-environment happy-dom
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

// No vitest globals in this workspace, so testing-library's automatic
// afterEach cleanup never registers.
afterEach(cleanup)

// useMailboxes and useSendableIdentities resolve membership, mailbox, domain
// and aliases in ONE joined live query each (no client-side Map stitching).
// These tests mount the real hooks against real TanStack DB collections so
// the join conditions and the user predicate actually execute — a fixture
// that never runs the query would certify a filter matching zero rows
// (the §3.2 bug class).

const h = vi.hoisted(() => ({
    domains: [{ id: 'd1', domain: 'acme.com' }],
    mailboxes: [
        {
            id: 'mb_me',
            address: 'alice',
            domain: 'd1',
            display_name: '',
            name: '',
            type: 'personal',
            created: '2024-01-01T00:00:00Z',
            updated: '',
        },
        {
            id: 'mb_support',
            address: 'support',
            domain: 'd1',
            display_name: 'Support',
            name: '',
            type: 'shared',
            created: '2024-02-01T00:00:00Z',
            updated: '',
        },
        // Bob's shared mailbox: alice holds no membership row on it, so the
        // membership predicate must exclude it from every result below.
        // Deliberately SHARED-typed: a personal-typed decoy would hide behind
        // splitMailboxes' find-first and a lost predicate would stay green.
        {
            id: 'mb_bob',
            address: 'ops',
            domain: 'd1',
            display_name: 'Ops',
            name: '',
            type: 'shared',
            created: '2024-01-15T00:00:00Z',
            updated: '',
        },
        // A mailbox whose domain row is gone: the domain inner join must drop
        // it from sendable identities even though alice is a member.
        {
            id: 'mb_orphan',
            address: 'ghost',
            domain: 'd_missing',
            display_name: '',
            name: '',
            type: 'shared',
            created: '2024-03-01T00:00:00Z',
            updated: '',
        },
    ],
    members: [
        { id: 'm1', mailbox: 'mb_me', user: 'u1', role: 'owner', created: '', updated: '' },
        { id: 'm2', mailbox: 'mb_support', user: 'u1', role: 'member', created: '', updated: '' },
        { id: 'm3', mailbox: 'mb_bob', user: 'u2', role: 'owner', created: '', updated: '' },
        { id: 'm4', mailbox: 'mb_orphan', user: 'u1', role: 'member', created: '', updated: '' },
    ],
    aliases: [
        { id: 'a1', mailbox: 'mb_support', address: 'help', created: '', updated: '' },
        { id: 'a2', mailbox: 'mb_support', address: 'sos', created: '', updated: '' },
        // Bob's alias: must never surface through alice's identities.
        { id: 'a3', mailbox: 'mb_bob', address: 'bobby', created: '', updated: '' },
    ],
}))

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: { id: 'u1' }, isLoggedIn: true }),
}))

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { createCollection, localOnlyCollectionOptions } = await import('@tanstack/db')
    const mk = (id: string, initialData: { id: string }[]) =>
        createCollection(
            localOnlyCollectionOptions({ id, getKey: (r: { id: string }) => r.id, initialData })
        )
    const stores: Record<string, unknown> = {
        mail_domains: mk('mail_domains', h.domains),
        mail_mailboxes: mk('mail_mailboxes', h.mailboxes),
        mail_mailbox_members: mk('mail_mailbox_members', h.members),
        mail_mailbox_aliases: mk('mail_mailbox_aliases', h.aliases),
    }
    return {
        useStore: (...names: string[]) => names.map(n => stores[n]),
    }
})

import { useMailboxes } from '~/tinycld/mail/hooks/useMailboxes'
import { useSendableIdentities } from '~/tinycld/mail/hooks/useSendableIdentities'

test('useMailboxes joins memberships to mailboxes for the signed-in user only', async () => {
    const { result } = renderHook(() => useMailboxes())

    await waitFor(() => expect(result.current.personal).not.toBeNull())

    expect(result.current.personal?.id).toBe('mb_me')
    // mb_bob is excluded by the membership predicate; mb_orphan is still a
    // membership (domains are not this hook's concern).
    expect(result.current.shared.map(m => m.id)).toEqual(['mb_support', 'mb_orphan'])
})

test('useSendableIdentities: one identity per reachable mailbox, aliases grouped, personal first', async () => {
    const { result } = renderHook(() => useSendableIdentities())

    await waitFor(() => expect(result.current.length).toBeGreaterThan(0))

    // mb_orphan (dead domain) and mb_bob (not a member) must both be absent.
    expect(result.current.map(i => i.mailboxId)).toEqual(['mb_me', 'mb_support'])

    const personal = result.current[0]
    expect(personal.primaryAddress).toBe('alice@acme.com')
    expect(personal.aliases).toEqual([])

    const support = result.current[1]
    expect(support.primaryAddress).toBe('support@acme.com')
    expect(support.mailboxDisplayName).toBe('Support')
    expect(support.aliases.map(a => a.address).sort()).toEqual(['help@acme.com', 'sos@acme.com'])
})
