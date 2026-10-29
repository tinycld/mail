import { log } from '@tinycld/core/lib/logger'
import { ClientResponseError } from 'pocketbase'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { logSetupFailure } from '~/tinycld/mail/setup/setup-logic'

function refusal(status: number) {
    return new ClientResponseError({
        url: '/api/mail/send',
        status,
        response: { message: 'DKIM not verified' },
    })
}

describe('logSetupFailure', () => {
    afterEach(() => {
        vi.restoreAllMocks()
    })

    // The step shows the server's refusal under its button, and the refusal
    // is the org's own setup state, so it must not become a Sentry warning.
    it('logs a server refusal below warning level', () => {
        const warn = vi.spyOn(log, 'warn')
        const info = vi.spyOn(log, 'info')

        logSetupFailure('mail.setup.test-message', refusal(403))

        expect(warn).not.toHaveBeenCalled()
        expect(info).toHaveBeenCalledTimes(1)
    })

    it('keeps a server fault a warning', () => {
        const warn = vi.spyOn(log, 'warn')

        logSetupFailure('mail.setup.verify', refusal(500))

        expect(warn).toHaveBeenCalledTimes(1)
    })
})
