// @vitest-environment happy-dom
import { cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, expect, test, vi } from 'vitest'

afterEach(cleanup)

// The membership collection here is on demand, as in the app: it records the
// filter of each load, and no load ends until the test releases it. The other
// collections hold their rows from the start.
const h = vi.hoisted(() => {
    let release = () => {}
    const ready = new Promise<void>(resolve => {
        release = resolve
    })
    return { loads: [] as string[], ready, release: () => release() }
})

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
    type MemberRow = { id: string; mailbox: string; group: string; user: string; role: string }
    const memberRows: MemberRow[] = [
        { id: 'grant', mailbox: 'mb_support', group: 'g_sales', user: '', role: 'member' },
        { id: 'ann_d', mailbox: 'mb_support', group: 'g_sales', user: 'ann', role: 'member' },
        // Zed is on an inbox Sales does not grant: deleting Sales must not load his row.
        { id: 'zed_x', mailbox: 'mb_other', group: '', user: 'zed', role: 'member' },
    ]
    const members = createCollection<MemberRow, string>({
        id: 'mail_mailbox_members',
        getKey: r => r.id,
        syncMode: 'on-demand',
        autoIndex: 'eager',
        defaultIndexType: BasicIndex,
        sync: {
            sync: ({ begin, write, commit, markReady }) => {
                const written = new Set<string>()
                markReady()
                return {
                    loadSubset: async options => {
                        h.loads.push(JSON.stringify(options.where ?? null))
                        await h.ready
                        begin()
                        for (const row of memberRows) {
                            if (written.has(row.id)) continue
                            written.add(row.id)
                            write({ type: 'insert', value: row })
                        }
                        await commit()
                    },
                }
            },
        },
    })
    const stores: Record<string, unknown> = {
        mail_domains: mk('mail_domains', [
            { id: 'd_q', domain: 'q.example', coverage: 'groups', coverage_groups: ['g_sales'] },
        ]),
        mail_identities: mk('mail_identities', [
            { id: 'i_ann', user: 'ann', inbox: 'mb_ann', email_name: 'ann', retired: false },
        ]),
        mail_addresses: mk('mail_addresses', [
            {
                id: 'a_ann',
                local_part: 'ann',
                domain: 'd_q',
                mailbox: 'mb_ann',
                source: 'rule',
                is_primary: false,
                via_group: 'g_sales',
            },
        ]),
        users: mk('users', [
            { id: 'ann', name: 'Ann', email: 'ann@tinycld.org' },
            { id: 'zed', name: 'Zed', email: 'zed@tinycld.org' },
        ]),
        mail_mailboxes: mk('mail_mailboxes', [
            { id: 'mb_support', type: 'shared', address: 'support', domain: 'd_q' },
            { id: 'mb_other', type: 'shared', address: 'other', domain: 'd_q' },
        ]),
        mail_mailbox_members: members,
        group_members: mk('group_members', [{ id: 'gm1', group: 'g_sales', user: 'ann' }]),
    }
    return { useStore: (...names: string[]) => names.map(n => stores[n]) }
})

import GroupRemovalNotice from '~/tinycld/mail/settings/GroupRemovalNotice'

test('the notice waits for both of its queries before it shows text', async () => {
    const { container, findByText } = render(<GroupRemovalNotice groupId="g_sales" userId="ann" />)
    // The address query is ready at once; the inbox query is not. Text shown
    // now would name only the address.
    await new Promise(resolve => setTimeout(resolve, 50))
    expect(container.textContent).toBe('')
    h.release()
    expect(
        await findByText(
            'Ann loses the address ann@q.example. Ann also loses access to support@q.example.'
        )
    ).toBeTruthy()
})

test('deleting the group loads only the member rows of the inboxes it grants', async () => {
    h.release()
    h.loads.length = 0
    const { findByText } = render(<GroupRemovalNotice groupId="g_sales" userId={null} />)
    expect(
        await findByText(
            'Members lose the address ann@q.example. Members also lose access to support@q.example.'
        )
    ).toBeTruthy()
    await waitFor(() => expect(h.loads.length).toBeGreaterThan(0))
    // Each load names the group or the inboxes: none asks for every person row.
    for (const where of h.loads) {
        expect(where.includes('"group"') || where.includes('"mailbox"'), where).toBe(true)
    }
})
