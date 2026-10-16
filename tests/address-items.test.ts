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
                groupName: null,
            },
            {
                id: 'm',
                localPart: 'admin',
                domainName: 'tinycld.org',
                source: 'manual',
                isPrimary: false,
                rank: 1,
                groupName: null,
            },
            {
                id: 'p',
                localPart: 'nathan',
                domainName: 'tinycld.org',
                source: 'manual',
                isPrimary: true,
                rank: 1,
                groupName: null,
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

    it('names the group that gave a rule address', () => {
        const items = toAddressItems([
            {
                id: 'g',
                localPart: 'ann',
                domainName: 'q.example',
                source: 'rule',
                isPrimary: false,
                rank: 1,
                groupName: 'Sales',
            },
            {
                id: 'd',
                localPart: 'ann',
                domainName: 'r.example',
                source: 'rule',
                isPrimary: false,
                rank: 2,
                groupName: null,
            },
        ])
        expect(items.map(i => i.badge)).toEqual(['From group Sales', 'From the domain rule'])
    })
})
