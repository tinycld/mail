import { describe, expect, it } from 'vitest'
import { toAddressItems } from '~/tinycld/mail/settings/address-items'

describe('toAddressItems', () => {
    it('labels main and rule addresses and only lets hand-added ones be removed', () => {
        const items = toAddressItems([
            {
                id: 'r',
                localPart: 'nathan',
                domainName: 'tinycld.com',
                source: 'rule',
                isPrimary: false,
                rank: 2,
            },
            {
                id: 'm',
                localPart: 'admin',
                domainName: 'tinycld.org',
                source: 'manual',
                isPrimary: false,
                rank: 1,
            },
            {
                id: 'p',
                localPart: 'nathan',
                domainName: 'tinycld.org',
                source: 'manual',
                isPrimary: true,
                rank: 1,
            },
        ])
        expect(items).toEqual([
            { id: 'p', email: 'nathan@tinycld.org', badge: 'Main address', canRemove: false },
            { id: 'm', email: 'admin@tinycld.org', badge: null, canRemove: true },
            {
                id: 'r',
                email: 'nathan@tinycld.com',
                badge: 'From the domain rule',
                canRemove: false,
            },
        ])
    })
})
