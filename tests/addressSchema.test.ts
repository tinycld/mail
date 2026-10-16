import { describe, expect, it } from 'vitest'
import { addressSchemaFor } from '~/tinycld/mail/settings/addressSchema'

const schema = addressSchemaFor({
    taken: new Set(['help@d_org']),
    reservedNames: new Set(['holly']),
    ruleDomainIds: new Set(['d_com']),
})

describe('addressSchemaFor', () => {
    it('accepts a free address and lowercases it', () => {
        const r = schema.safeParse({ local_part: ' Sales ', domain: 'd_org' })
        expect(r.success && r.data).toEqual({ local_part: 'sales', domain: 'd_org' })
    })
    it('refuses an address that exists on that domain', () => {
        expect(schema.safeParse({ local_part: 'help', domain: 'd_org' }).success).toBe(false)
    })
    it("refuses a person's email name on a domain with a rule", () => {
        expect(schema.safeParse({ local_part: 'holly', domain: 'd_com' }).success).toBe(false)
    })
    it("allows a person's email name on a manual domain", () => {
        expect(schema.safeParse({ local_part: 'holly', domain: 'd_org' }).success).toBe(true)
    })
    it('requires a domain and a valid local part', () => {
        expect(schema.safeParse({ local_part: 'a b', domain: 'd_org' }).success).toBe(false)
        expect(schema.safeParse({ local_part: 'ok', domain: '' }).success).toBe(false)
    })
})
