import { describe, expect, it } from 'vitest'
import { coverageChangePreview } from '~/tinycld/mail/settings/coverage-preview'

const people = [
    { emailName: 'nathan', inbox: 'mb_n' },
    { emailName: 'nas', inbox: 'mb_s' },
]

describe('coverageChangePreview', () => {
    it('everyone: lists the addresses people will get', () => {
        expect(
            coverageChangePreview({
                target: 'everyone',
                domainName: 'tinycld.com',
                people,
                addressesOnDomain: [{ localPart: 'nathan', mailbox: 'mb_n', source: 'manual' }],
            })
        ).toEqual({ adds: ['nas@tinycld.com'], removes: [] })
    })

    it('manual: lists the rule addresses that will stop working', () => {
        expect(
            coverageChangePreview({
                target: 'manual',
                domainName: 'tinycld.com',
                people,
                addressesOnDomain: [
                    { localPart: 'nathan', mailbox: 'mb_n', source: 'rule' },
                    { localPart: 'sales', mailbox: 'mb_x', source: 'manual' },
                ],
            })
        ).toEqual({ adds: [], removes: ['nathan@tinycld.com'] })
    })
})
