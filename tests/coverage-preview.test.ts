import { describe, expect, it } from 'vitest'
import { coverageChangePreview, coveragePreviewText } from '~/tinycld/mail/settings/coverage-preview'

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

    // The server's live identity rule (server/rule_addresses.go) skips an
    // identity whose inbox is gone, so the preview must too.
    it('everyone: skips a person with no inbox', () => {
        expect(
            coverageChangePreview({
                target: 'everyone',
                domainName: 'tinycld.com',
                people: [...people, { emailName: 'orphan', inbox: '' }],
                addressesOnDomain: [],
            })
        ).toEqual({ adds: ['nas@tinycld.com', 'nathan@tinycld.com'], removes: [] })
    })
})

const addresses = (n: number) =>
    Array.from({ length: n }, (_, i) => `p${i + 1}@tinycld.com`)

describe('coveragePreviewText', () => {
    it('lists every address when there are five or fewer', () => {
        expect(coveragePreviewText({ adds: addresses(2), removes: [] })).toBe(
            'Adds 2: p1@tinycld.com, p2@tinycld.com'
        )
    })

    it('lists the first five addresses and counts the rest', () => {
        expect(coveragePreviewText({ adds: addresses(7), removes: [] })).toBe(
            'Adds 7: p1@tinycld.com, p2@tinycld.com, p3@tinycld.com, p4@tinycld.com, p5@tinycld.com and 2 more'
        )
    })

    it('counts and shortens the addresses that stop receiving mail', () => {
        expect(coveragePreviewText({ adds: [], removes: addresses(6) })).toBe(
            '6 addresses stop receiving mail: p1@tinycld.com, p2@tinycld.com, p3@tinycld.com, p4@tinycld.com, p5@tinycld.com and 1 more'
        )
    })

    it('uses the singular for one address that stops receiving mail', () => {
        expect(coveragePreviewText({ adds: [], removes: addresses(1) })).toBe(
            '1 address stops receiving mail: p1@tinycld.com'
        )
    })

    it('says when nothing changes', () => {
        expect(coveragePreviewText({ adds: [], removes: [] })).toBe('No addresses change.')
    })
})
