import { describe, expect, it } from 'vitest'
import {
    currentChoice,
    domainTabs,
    initialChoice,
    OWN_DOMAIN,
    showDomainTabs,
    showOwnDomainForm,
} from '~/tinycld/mail/setup/domain-choice'

const HOSTED = { contributorSlug: 'hosting-ui', label: 'Built-in domain' }

describe('domainTabs', () => {
    it('lists each contributed option before the own-domain tab', () => {
        expect(domainTabs([HOSTED])).toEqual([
            { value: 'hosting-ui', label: 'Built-in domain' },
            { value: OWN_DOMAIN, label: 'Your own domain' },
        ])
    })

    it('names an unlabelled contribution after its package', () => {
        expect(domainTabs([{ contributorSlug: 'acme-domains' }])[0]?.label).toBe('acme-domains')
    })
})

describe('initialChoice', () => {
    it('opens the own-domain form when no package offers a domain', () => {
        expect(initialChoice(domainTabs([]))).toBe(OWN_DOMAIN)
    })

    it('opens the first contributed option when a package offers one', () => {
        expect(initialChoice(domainTabs([HOSTED]))).toBe('hosting-ui')
    })
})

describe('currentChoice', () => {
    const tabs = domainTabs([HOSTED])

    it('keeps the tab picked on an earlier visit', () => {
        expect(currentChoice(tabs, OWN_DOMAIN)).toBe(OWN_DOMAIN)
    })

    it('opens the default tab when nothing was picked', () => {
        expect(currentChoice(tabs, null)).toBe('hosting-ui')
    })

    it('falls back to the default when the picked tab is gone', () => {
        expect(currentChoice(tabs, 'removed-package')).toBe('hosting-ui')
    })
})

describe('showDomainTabs', () => {
    it('shows the bar only when there is more than one choice', () => {
        expect(showDomainTabs(domainTabs([]))).toBe(false)
        expect(showDomainTabs(domainTabs([HOSTED]))).toBe(true)
    })
})

describe('showOwnDomainForm', () => {
    it('shows the form only for the own-domain choice', () => {
        expect(showOwnDomainForm(OWN_DOMAIN)).toBe(true)
        expect(showOwnDomainForm('hosting-ui')).toBe(false)
    })
})
