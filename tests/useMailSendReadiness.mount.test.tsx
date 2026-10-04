// @vitest-environment happy-dom

import { cleanup, renderHook, waitFor } from '@testing-library/react'
import { __resetCoreConfigForTests, configureCore } from '@tinycld/core/lib/core-config'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

afterEach(cleanup)

const h = vi.hoisted(() => ({
    captureMessage: vi.fn(),
    addBreadcrumb: vi.fn(),
    domains: [{ id: 'd1', domain: 'acme.com', verified: true }],
    // The domain row is not in the store, so the hook stays in its loading
    // suppression for as long as the test runs.
    mailboxes: [
        {
            id: 'mb_me',
            address: 'alice',
            domain: 'd_pending',
            type: 'personal',
            created: '',
            updated: '',
        },
    ],
    members: [{ id: 'm1', mailbox: 'mb_me', user: 'u1', role: 'owner', created: '', updated: '' }],
}))

vi.mock('@tinycld/core/lib/sentry', () => ({
    captureMessageToSentry: h.captureMessage,
    addBreadcrumbToSentry: h.addBreadcrumb,
    captureExceptionToSentry: vi.fn(),
}))

vi.mock('@tinycld/core/lib/auth', () => ({
    useAuth: () => ({ user: { id: 'u1' }, isLoggedIn: true }),
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
        mail_mailboxes: mk('mail_mailboxes', h.mailboxes),
        mail_mailbox_members: mk('mail_mailbox_members', h.members),
    }
    return {
        useStore: (...names: string[]) => names.map(n => stores[n]),
    }
})

import { useMailSendReadiness } from '~/tinycld/mail/hooks/useMailSendReadiness'

beforeEach(() => {
    __resetCoreConfigForTests()
    // The release default: only warn and above become Sentry events.
    configureCore({ brandName: 'T', serverShortcuts: {}, logLevel: 'warn' })
    h.captureMessage.mockClear()
    h.addBreadcrumb.mockClear()
})

// Every compose mount passes through the loading state while its queries
// resolve. That is expected, so it must stay a breadcrumb, not a Sentry event.
test('the loading suppression is breadcrumbed, not reported as a Sentry event', async () => {
    const { result } = renderHook(() => useMailSendReadiness())

    await waitFor(() =>
        expect(h.addBreadcrumb).toHaveBeenCalledWith(
            'mail-send-readiness',
            'info',
            'blocker-suppressed-loading',
            expect.objectContaining({ loading: ['domains'], domainId: 'd_pending' })
        )
    )
    expect(result.current).toEqual({ mailboxId: 'mb_me', blocker: null, message: null })
    expect(h.captureMessage).not.toHaveBeenCalled()
})
