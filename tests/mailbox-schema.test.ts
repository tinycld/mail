import { describe, expect, it } from 'vitest'
import { sharedMailboxSchemaFor } from '~/tinycld/mail/settings/mailbox-schema'

const schema = sharedMailboxSchemaFor(
    {
        taken: new Set(['help@d_org']),
        reservedNames: new Set(['holly']),
        ruleDomainIds: new Set(['d_com']),
    },
    new Map([
        ['d_com', 'example.com'],
        ['d_org', 'example.org'],
    ])
)

function addressError(value: { address: string; domains: string[] }) {
    const r = schema.safeParse({ ...value, display_name: 'Team' })
    if (r.success) return null
    return r.error.issues.find(i => i.path[0] === 'address')?.message ?? null
}

describe('sharedMailboxSchemaFor', () => {
    it('accepts an address that is free on every chosen domain', () => {
        const r = schema.safeParse({
            address: ' Support ',
            domains: ['d_com', 'd_org'],
            display_name: 'Team',
        })
        expect(r.success && r.data.address).toBe('support')
    })
    it('names the full address that already exists', () => {
        expect(addressError({ address: 'help', domains: ['d_com', 'd_org'] })).toBe(
            'help@example.org already exists'
        )
    })
    it('allows that address when its domain is not chosen', () => {
        expect(addressError({ address: 'help', domains: ['d_com'] })).toBeNull()
    })
    it("refuses a person's email name on a chosen domain with a rule", () => {
        expect(addressError({ address: 'holly', domains: ['d_org', 'd_com'] })).toMatch(
            /^holly@example\.com is a person's email name/
        )
    })
    it('requires at least one domain', () => {
        const r = schema.safeParse({ address: 'ok', domains: [], display_name: 'Team' })
        expect(r.success).toBe(false)
    })
})
