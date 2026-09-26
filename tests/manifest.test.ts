import { describe, expect, it } from 'vitest'
import manifest from '../manifest'

describe('mail manifest', () => {
    it('declares required identifiers', () => {
        expect(manifest.name).toBe('Mail')
        expect(manifest.slug).toBe('mail')
        expect(manifest.version).toMatch(/^\d+\.\d+\.\d+/)
    })

    it('points routes directory at screens', () => {
        expect(manifest.routes?.directory).toBe('screens')
    })

    it('declares migrations, collections, and seed', () => {
        expect(manifest.migrations?.directory).toBe('pb-migrations')
        expect(manifest.collections?.register).toBe('collections')
        expect(manifest.collections?.types).toBe('types')
        expect(manifest.seed?.script).toBe('seed')
    })

    it('declares a nav entry', () => {
        expect(manifest.nav?.label).toBe('Mail')
        expect(manifest.nav?.icon).toBe('mail')
        expect(typeof manifest.nav?.order).toBe('number')
    })

    it('declares a server module', () => {
        expect(manifest.server?.package).toBe('server')
        expect(manifest.server?.module).toBe('tinycld.org/packages/mail')
    })

    it('declares settings panels', () => {
        expect(Array.isArray(manifest.settings)).toBe(true)
        expect(manifest.settings?.length).toBeGreaterThan(0)
    })

    it('declares the setup wizard steps and the domain options slot', () => {
        expect(manifest.setupSteps).toEqual([
            {
                id: 'email-domain',
                label: 'Email domain',
                module: 'setup/EmailDomainStep',
                order: 'a2V',
            },
            { id: 'address', label: 'Your address', module: 'setup/AddressStep', order: 'a2k' },
        ])
        expect(manifest.slots).toContain('setup-domain-options')
    })

    // Both steps need a mail provider, which core's Email sending step (a2)
    // sets up, and both come before the team invite (a3).
    it('orders the setup steps after Email sending and before the team', () => {
        for (const step of manifest.setupSteps ?? []) {
            expect(step.order > 'a2').toBe(true)
            expect(step.order < 'a3').toBe(true)
        }
    })
})
