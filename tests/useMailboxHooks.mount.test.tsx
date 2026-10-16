// @vitest-environment happy-dom
import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

// No vitest globals in this workspace, so testing-library's automatic
// afterEach cleanup never registers.
afterEach(cleanup)

// useMailboxes and useSendableIdentities resolve membership, mailbox, address
// and domain in ONE joined live query each (no client-side Map stitching).
// These tests mount the real hooks against real TanStack DB collections so
// the join conditions and the user predicate actually execute — a fixture
// that never runs the query would certify a filter matching zero rows
// (the §3.2 bug class).

const h = vi.hoisted(() => ({
    domains: [{ id: 'd1', domain: 'acme.com', rank: 1, verified: true }],
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
        // A mailbox whose only address points at a domain row that is gone:
        // the domain inner join must drop it from sendable identities even
        // though alice is a member.
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
        // A membership whose mailbox row is gone: useMailboxes left-joins the
        // mailbox, so it must drop this row itself.
        { id: 'm5', mailbox: 'mb_gone', user: 'u1', role: 'member', created: '', updated: '' },
    ],
    addresses: [
        {
            id: 'ad_me',
            mailbox: 'mb_me',
            domain: 'd1',
            local_part: 'alice',
            source: 'manual',
            is_primary: true,
            created: '',
            updated: '',
        },
        {
            id: 'ad_support',
            mailbox: 'mb_support',
            domain: 'd1',
            local_part: 'support',
            source: 'manual',
            is_primary: true,
            created: '',
            updated: '',
        },
        {
            id: 'ad_help',
            mailbox: 'mb_support',
            domain: 'd1',
            local_part: 'help',
            source: 'manual',
            is_primary: false,
            created: '',
            updated: '',
        },
        {
            id: 'ad_sos',
            mailbox: 'mb_support',
            domain: 'd1',
            local_part: 'sos',
            source: 'manual',
            is_primary: false,
            created: '',
            updated: '',
        },
        {
            id: 'ad_bob',
            mailbox: 'mb_bob',
            domain: 'd1',
            local_part: 'ops',
            source: 'manual',
            is_primary: true,
            created: '',
            updated: '',
        },
        // Bob's second address: must never surface through alice's identities.
        {
            id: 'ad_bobby',
            mailbox: 'mb_bob',
            domain: 'd1',
            local_part: 'bobby',
            source: 'manual',
            is_primary: false,
            created: '',
            updated: '',
        },
        {
            id: 'ad_orphan',
            mailbox: 'mb_orphan',
            domain: 'd_missing',
            local_part: 'ghost',
            source: 'manual',
            is_primary: true,
            created: '',
            updated: '',
        },
    ],
    identities: [],
}))

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: { id: 'u1' }, isLoggedIn: true }),
}))

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { BasicIndex, createCollection, localOnlyCollectionOptions } = await import(
        '@tanstack/db'
    )
    // Index like the real collections (core/lib/pocketbase.ts) so joins do not fall back to a scan.
    const mk = (id: string, initialData: { id: string }[]) =>
        createCollection({
            ...localOnlyCollectionOptions({ id, getKey: (r: { id: string }) => r.id, initialData }),
            autoIndex: 'eager',
            defaultIndexType: BasicIndex,
        })
    const stores: Record<string, unknown> = {
        mail_domains: mk('mail_domains', h.domains),
        mail_mailboxes: mk('mail_mailboxes', h.mailboxes),
        mail_mailbox_members: mk('mail_mailbox_members', h.members),
        mail_addresses: mk('mail_addresses', h.addresses),
        mail_identities: mk('mail_identities', h.identities),
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
    // membership (domains are not this hook's concern); mb_gone has no
    // mailbox row.
    expect(result.current.shared.map(m => m.id)).toEqual(['mb_support', 'mb_orphan'])
})

test('useSendableIdentities: one identity per reachable mailbox, addresses grouped, personal first', async () => {
    const { result } = renderHook(() => useSendableIdentities())

    await waitFor(() => expect(result.current.length).toBeGreaterThan(0))

    // mb_orphan (dead domain) and mb_bob (not a member) must both be absent.
    expect(result.current.map(i => i.mailboxId)).toEqual(['mb_me', 'mb_support'])

    const personal = result.current[0]
    expect(personal.addresses.map(a => a.email)).toEqual(['alice@acme.com'])
    expect(personal.defaultAddressId).toBe('ad_me')
    expect(personal.isPersonal).toBe(true)

    const support = result.current[1]
    expect(support.mailboxDisplayName).toBe('Support')
    expect(support.defaultAddressId).toBe('ad_support')
    expect(support.isPersonal).toBe(false)
    expect(support.addresses.map(a => a.email).sort()).toEqual([
        'help@acme.com',
        'sos@acme.com',
        'support@acme.com',
    ])

    const allEmails = result.current.flatMap(i => i.addresses.map(a => a.email))
    expect(allEmails.some(e => e.startsWith('bobby@') || e.startsWith('ops@'))).toBe(false)
})
