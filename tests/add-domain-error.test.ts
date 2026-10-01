import { ClientResponseError } from 'pocketbase'
import { describe, expect, it } from 'vitest'
import { addDomainErrorMessage } from '~/tinycld/mail/settings/add-domain-error'

function responseError(status: number, message: string, data: Record<string, unknown> = {}) {
    return new ClientResponseError({ status, response: { status, message, data } })
}

describe('addDomainErrorMessage', () => {
    it('shows a plain refusal under the field', () => {
        const error = responseError(409, 'That domain is already configured on this host.')
        expect(addDomainErrorMessage(error)).toBe('That domain is already configured on this host.')
    })
    it('shows the domain field error from a validation failure', () => {
        const error = responseError(400, 'Failed to add.', {
            domain: { code: 'invalid', message: 'Must be a domain.' },
        })
        expect(addDomainErrorMessage(error)).toBe('Must be a domain.')
    })
    it("prefers the caller's own message", () => {
        const error = responseError(503, 'Unavailable.')
        expect(addDomainErrorMessage(error, () => 'Set up email sending first.')).toBe(
            'Set up email sending first.'
        )
    })
    it('keeps the server message when the caller has none', () => {
        const error = responseError(409, 'Taken.')
        expect(addDomainErrorMessage(error, () => null)).toBe('Taken.')
    })
})
