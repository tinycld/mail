// @vitest-environment happy-dom
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

const h = vi.hoisted(() => ({
    identities: [
        {
            id: 'i1',
            user: 'u1',
            email_name: 'nathan',
            inbox: 'mb1',
            default_from: 'a1',
            retired: false,
        },
    ],
    addresses: [] as { id: string }[],
    updated: [] as Record<string, unknown>[],
    inserted: [] as Record<string, unknown>[],
    rejectUpdate: null as unknown,
}))

beforeEach(() => {
    h.rejectUpdate = null
})

afterEach(() => {
    cleanup()
    h.updated.length = 0
    h.inserted.length = 0
})

vi.mock('@tinycld/core/lib/pocketbase', async () => {
    const { BasicIndex, createCollection, localOnlyCollectionOptions } = await import(
        '@tanstack/db'
    )
    const mk = (
        id: string,
        initialData: { id: string }[],
        hooks: Record<string, (args: never) => Promise<void>>
    ) =>
        createCollection({
            ...localOnlyCollectionOptions({
                id,
                getKey: (r: { id: string }) => r.id,
                initialData,
                ...hooks,
            }),
            autoIndex: 'eager',
            defaultIndexType: BasicIndex,
        })
    const stores: Record<string, unknown> = {
        mail_identities: mk('mail_identities', h.identities, {
            onUpdate: async ({ transaction }: never) => {
                if (h.rejectUpdate) throw h.rejectUpdate
                for (const m of (transaction as { mutations: { modified: never }[] }).mutations)
                    h.updated.push(m.modified)
            },
        }),
        mail_addresses: mk('mail_addresses', h.addresses, {
            onInsert: async ({ transaction }: never) => {
                for (const m of (transaction as { mutations: { modified: never }[] }).mutations)
                    h.inserted.push(m.modified)
            },
        }),
    }
    return { useStore: (...names: string[]) => names.map(n => stores[n]) }
})

import { EmailNameForm } from '~/tinycld/mail/settings/EmailNameForm'

const identity = { id: 'i1', emailName: 'nathan', inbox: 'mb1', defaultFrom: 'a1' }
const addresses = [
    { id: 'a1', localPart: 'nathan', domainId: 'd_com', domainName: 'tinycld.com', source: 'rule' },
    { id: 'a2', localPart: 'nathan', domainId: 'd_q', domainName: 'q.example', source: 'rule' },
    { id: 'a3', localPart: 'sales', domainId: 'd_q', domainName: 'q.example', source: 'manual' },
].map(a => ({ ...a, isPrimary: false, rank: 1, groupName: null }))

function renderForm(onDone = vi.fn(), defaultFrom = 'a1') {
    return render(
        <QueryClientProvider client={new QueryClient()}>
            <EmailNameForm
                identity={{ ...identity, defaultFrom }}
                addresses={addresses}
                onDone={onDone}
            />
        </QueryClientProvider>
    )
}

async function rename(
    ui: ReturnType<typeof renderForm>,
    name: string,
    opts: { keepOld: boolean } = { keepOld: true }
) {
    await ui.findByText('Email name')
    // The react-native stub's TextInput listens for a DOM `input` event.
    const input = ui.container.querySelector('[testid="email_name"]') as HTMLElement
    await act(async () => {
        fireEvent.input(input, { target: { value: name } })
    })
    if (!opts.keepOld) {
        await act(async () => {
            fireEvent.click(
                ui.container.querySelector(
                    '[accessibilitylabel="Keep the old addresses as extra addresses"]'
                ) as HTMLElement
            )
        })
    }
    await act(async () => {
        fireEvent.click(await ui.findByText('Rename'))
    })
}

it('renames and adds the old rule addresses back as manual addresses', async () => {
    const onDone = vi.fn()
    await rename(renderForm(onDone), 'anna')
    await waitFor(() => expect(h.inserted).toHaveLength(2))
    expect(h.updated[0]).toMatchObject({ id: 'i1', email_name: 'anna' })
    expect(h.inserted).toEqual([
        expect.objectContaining({
            local_part: 'nathan',
            domain: 'd_com',
            mailbox: 'mb1',
            source: 'manual',
        }),
        expect.objectContaining({ local_part: 'nathan', domain: 'd_q', source: 'manual' }),
    ])
    await waitFor(() => expect(onDone).toHaveBeenCalled())
})

it('points Default From at the kept address', async () => {
    await rename(renderForm(), 'bea')
    await waitFor(() => expect(h.updated).toHaveLength(2))
    const kept = h.inserted.find(r => r.domain === 'd_com')
    expect(h.updated[1]).toMatchObject({ id: 'i1', default_from: kept?.id })
})

it('renames without adding addresses when keep-old is off', async () => {
    await rename(renderForm(), 'cara', { keepOld: false })
    await waitFor(() => expect(h.updated).toHaveLength(1))
    expect(h.updated[0]).toMatchObject({ email_name: 'cara' })
    expect(h.inserted).toHaveLength(0)
})

it('shows the server field error under the name', async () => {
    // Like PocketBase's ClientResponseError: an Error carrying the parsed body.
    h.rejectUpdate = Object.assign(new Error('Failed to update record.'), {
        response: {
            data: {
                email_name: { message: 'These addresses already use this name: anna@q.example' },
            },
        },
    })
    const ui = renderForm()
    await rename(ui, 'dana')
    expect(
        (await ui.findAllByText('These addresses already use this name: anna@q.example')).length
    ).toBeGreaterThan(0)
    expect(h.inserted).toHaveLength(0)
})
