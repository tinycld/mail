// @vitest-environment happy-dom

// Autosave fires on a timer, independent of any user action, so a known
// outage (offline, or server unreachable) must pause quietly — no toast, no
// Sentry noise — while useWritesAvailable says writes can't be saved. A real
// failure (validation, a rejected request while writes ARE available) must
// still reach Sentry and the caller's onError exactly as before.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { useConnectivityStore } from '@tinycld/core/lib/stores/connectivity-store'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const h = vi.hoisted(() => ({
    logError: vi.fn(),
    logInfo: vi.fn(),
    send: vi.fn(),
}))

vi.mock('@tinycld/core/lib/logger', () => ({
    log: { error: h.logError, info: h.logInfo, warn: vi.fn(), debug: vi.fn() },
}))

vi.mock('@tinycld/core/lib/pocketbase', () => ({
    PB_SERVER_ADDR: 'http://pb.test',
    pb: { send: h.send, authStore: { token: 'tok' } },
}))

import { useSaveDraft } from '~/tinycld/mail/hooks/useSaveDraft'

function wrapper() {
    const client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
    return ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={client}>{children}</QueryClientProvider>
    )
}

describe('useSaveDraft', () => {
    beforeEach(() => {
        useConnectivityStore.setState({ isOnline: true, isServerReachable: true })
        vi.clearAllMocks()
    })

    it('stays quiet when a save fails because the server is unreachable', async () => {
        useConnectivityStore.setState({ isServerReachable: false })
        h.send.mockRejectedValue(new TypeError('Failed to fetch'))
        const onError = vi.fn()

        const { result } = renderHook(() => useSaveDraft({ onError }), { wrapper: wrapper() })
        result.current.saveDraft({ mailbox_id: 'mb1', subject: 'draft' })

        await waitFor(() => expect(h.send).toHaveBeenCalled())
        await waitFor(() => expect(h.logInfo).toHaveBeenCalled())

        expect(h.logError).not.toHaveBeenCalled()
        expect(onError).not.toHaveBeenCalled()
    })

    it('stays quiet while offline too', async () => {
        useConnectivityStore.setState({ isOnline: false })
        h.send.mockRejectedValue(new TypeError('Failed to fetch'))
        const onError = vi.fn()

        const { result } = renderHook(() => useSaveDraft({ onError }), { wrapper: wrapper() })
        result.current.saveDraft({ mailbox_id: 'mb1', subject: 'draft' })

        await waitFor(() => expect(h.logInfo).toHaveBeenCalled())
        expect(h.logError).not.toHaveBeenCalled()
        expect(onError).not.toHaveBeenCalled()
    })

    it('still reports a real failure to Sentry and the caller while writes are available', async () => {
        h.send.mockRejectedValue(new Error('rule denied the write'))
        const onError = vi.fn()

        const { result } = renderHook(() => useSaveDraft({ onError }), { wrapper: wrapper() })
        result.current.saveDraft({ mailbox_id: 'mb1', subject: 'draft' })

        await waitFor(() => expect(h.logError).toHaveBeenCalled())

        expect(h.logError).toHaveBeenCalledWith('mail.draft.save', expect.any(Error), undefined)
        expect(onError).toHaveBeenCalledWith('rule denied the write')
        expect(h.logInfo).not.toHaveBeenCalled()
    })

    it('resumes reporting once the connection comes back', async () => {
        useConnectivityStore.setState({ isServerReachable: false })
        h.send.mockRejectedValue(new Error('still down'))
        const onError = vi.fn()

        const { result, rerender } = renderHook(() => useSaveDraft({ onError }), {
            wrapper: wrapper(),
        })
        result.current.saveDraft({ mailbox_id: 'mb1', subject: 'draft' })
        await waitFor(() => expect(h.logInfo).toHaveBeenCalledTimes(1))
        expect(h.logError).not.toHaveBeenCalled()

        useConnectivityStore.setState({ isServerReachable: true })
        h.send.mockRejectedValue(new Error('genuinely failed'))
        rerender()
        result.current.saveDraft({ mailbox_id: 'mb1', subject: 'draft' })

        await waitFor(() => expect(h.logError).toHaveBeenCalledTimes(1))
        expect(onError).toHaveBeenCalledWith('genuinely failed')
    })
})
