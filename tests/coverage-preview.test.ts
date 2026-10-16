import { describe, expect, it } from 'vitest'
import {
    coverageChangePreview,
    coveragePreviewText,
} from '~/tinycld/mail/settings/coverage-preview'

const people = [
    { userId: 'u_n', emailName: 'nathan', inbox: 'mb_n' },
    { userId: 'u_s', emailName: 'nas', inbox: 'mb_s' },
]

const noGroups = { targetGroups: [] as string[], memberships: new Map<string, string[]>() }

describe('coverageChangePreview', () => {
    it('everyone: lists the addresses people will get', () => {
        expect(
            coverageChangePreview({
                target: 'everyone',
                ...noGroups,
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
                ...noGroups,
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
                ...noGroups,
                domainName: 'tinycld.com',
                people: [...people, { userId: 'u_o', emailName: 'orphan', inbox: '' }],
                addressesOnDomain: [],
            })
        ).toEqual({ adds: ['nas@tinycld.com', 'nathan@tinycld.com'], removes: [] })
    })
})

// Same terms as the server's group scope (server/rule_addresses.go): only a
// live member of a chosen group gets a rule address; rule rows of anyone else
// stop working. A rule row belongs to the person whose inbox it delivers to.
describe('coverageChangePreview: groups', () => {
    const team = [
        { userId: 'u_ann', emailName: 'ann', inbox: 'mb_ann' },
        { userId: 'u_bob', emailName: 'bob', inbox: 'mb_bob' },
        { userId: 'u_cat', emailName: 'cat', inbox: 'mb_cat' },
        { userId: 'u_dan', emailName: 'dan', inbox: '' },
    ]
    const memberships = new Map([
        ['u_ann', ['g_sales']],
        ['u_bob', ['g_ops']],
        ['u_cat', ['g_sales', 'g_ops']],
        ['u_dan', ['g_sales']],
    ])

    it('adds only members without an address and removes non-members rule rows', () => {
        expect(
            coverageChangePreview({
                target: 'groups',
                targetGroups: ['g_sales'],
                memberships,
                domainName: 'q.example',
                people: team,
                addressesOnDomain: [
                    { localPart: 'cat', mailbox: 'mb_cat', source: 'rule' },
                    { localPart: 'bob', mailbox: 'mb_bob', source: 'rule' },
                    { localPart: 'help', mailbox: 'mb_bob', source: 'manual' },
                ],
            })
        ).toEqual({ adds: ['ann@q.example'], removes: ['bob@q.example'] })
    })

    it('switching from everyone lists every non-member rule row in removes', () => {
        expect(
            coverageChangePreview({
                target: 'groups',
                targetGroups: ['g_ops'],
                memberships,
                domainName: 'q.example',
                people: team,
                addressesOnDomain: [
                    { localPart: 'ann', mailbox: 'mb_ann', source: 'rule' },
                    { localPart: 'bob', mailbox: 'mb_bob', source: 'rule' },
                    { localPart: 'cat', mailbox: 'mb_cat', source: 'rule' },
                ],
            })
        ).toEqual({ adds: [], removes: ['ann@q.example'] })
    })

    // The form refuses a groups rule with no group, but the preview still
    // tells the truth about it.
    it('with no group: adds nobody and removes every rule row', () => {
        expect(
            coverageChangePreview({
                target: 'groups',
                targetGroups: [],
                memberships,
                domainName: 'q.example',
                people: team,
                addressesOnDomain: [
                    { localPart: 'ann', mailbox: 'mb_ann', source: 'rule' },
                    { localPart: 'cat', mailbox: 'mb_cat', source: 'rule' },
                    { localPart: 'help', mailbox: 'mb_bob', source: 'manual' },
                ],
            })
        ).toEqual({ adds: [], removes: ['ann@q.example', 'cat@q.example'] })
    })
})

const addresses = (n: number) => Array.from({ length: n }, (_, i) => `p${i + 1}@tinycld.com`)

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

    // A change of groups can add addresses and remove others at once.
    it('describes both the adds and the removes', () => {
        expect(coveragePreviewText({ adds: ['ann@q.example'], removes: ['bob@q.example'] })).toBe(
            'Adds 1: ann@q.example. 1 address stops receiving mail: bob@q.example'
        )
    })

    it('says when nothing changes', () => {
        expect(coveragePreviewText({ adds: [], removes: [] })).toBe('No addresses change.')
    })
})
