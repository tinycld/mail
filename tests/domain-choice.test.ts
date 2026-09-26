import { describe, expect, it } from 'vitest'
import { initialChoice, showOwnDomainForm } from '~/tinycld/mail/setup/domain-choice'

describe('initialChoice', () => {
    it('opens the own-domain form when no package offers a domain', () => {
        expect(initialChoice(false)).toBe('own')
    })

    it('asks first when a package offers a domain', () => {
        expect(initialChoice(true)).toBe('none')
    })
})

describe('showOwnDomainForm', () => {
    it('shows the form only for the own-domain choice', () => {
        expect(showOwnDomainForm('own')).toBe(true)
        expect(showOwnDomainForm('none')).toBe(false)
    })
})
