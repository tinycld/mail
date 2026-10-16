import { describe, expect, it } from 'vitest'
import {
    defaultFromAfterRename,
    emailNameSchema,
    keptAddresses,
    renameIncompleteMessage,
} from '~/tinycld/mail/settings/email-name'

const row = (
    id: string,
    localPart: string,
    domainId: string,
    source: string,
    isPrimary = false
) => ({
    id,
    localPart,
    domainId,
    domainName: `${domainId}.example`,
    source,
    isPrimary,
})

const rows = [
    row('a0', 'nathan', 'main', 'manual', true),
    row('a1', 'nathan', 'd1', 'rule'),
    row('a2', 'nathan', 'd2', 'rule'),
    row('a3', 'nathan', 'd3', 'manual'),
    row('a4', 'sales', 'd1', 'rule'),
]

describe('keptAddresses', () => {
    it('keeps the main address and the rule rows with the old local part', () => {
        expect(keptAddresses(rows, 'nathan', 'nate')).toEqual([
            expect.objectContaining({
                localPart: 'nathan',
                domainId: 'main',
                oldId: 'a0',
                isPrimary: true,
            }),
            expect.objectContaining({
                localPart: 'nathan',
                domainId: 'd1',
                oldId: 'a1',
                isPrimary: false,
            }),
            expect.objectContaining({
                localPart: 'nathan',
                domainId: 'd2',
                oldId: 'a2',
                isPrimary: false,
            }),
        ])
    })
    it('keeps a legacy main address that differs from both names', () => {
        const legacy = [row('a0', 'john.smith', 'main', 'manual', true)]
        expect(keptAddresses(legacy, 'john', 'jon')).toEqual([
            expect.objectContaining({ localPart: 'john.smith', oldId: 'a0', isPrimary: true }),
        ])
    })
    it('does not keep a main address equal to the new name', () => {
        const back = [row('a0', 'john', 'main', 'manual', true)]
        expect(keptAddresses(back, 'jon', 'john')).toEqual([])
    })
    it('does not keep a rule row with another local part', () => {
        expect(keptAddresses([row('a9', 'legacy', 'd1', 'rule')], 'nathan', 'nate')).toEqual([])
    })
    it('gives each kept address a new id and its full address', () => {
        const kept = keptAddresses(rows, 'nathan', 'nate')
        expect(new Set(kept.map(k => k.newId)).size).toBe(kept.length)
        expect(kept[1].email).toBe('nathan@d1.example')
    })
})

describe('defaultFromAfterRename', () => {
    const kept = keptAddresses(rows, 'nathan', 'nate')
    const newIdOf = (oldId: string) => kept.find(k => k.oldId === oldId)?.newId
    it('returns the new id when the default was a kept rule row', () => {
        expect(defaultFromAfterRename('a2', kept)).toBe(newIdOf('a2'))
    })
    it('returns null when the default is the main address, which keeps its id', () => {
        expect(defaultFromAfterRename('a0', kept)).toBeNull()
    })
    it('returns null when the default is a manual row', () => {
        expect(defaultFromAfterRename('a3', kept)).toBeNull()
    })
    it('returns null when the default is empty', () => {
        expect(defaultFromAfterRename('', kept)).toBeNull()
    })
    it('returns null when keep-old is off', () => {
        expect(defaultFromAfterRename('a1', [])).toBeNull()
    })
})

describe('renameIncompleteMessage', () => {
    it('is null when every step worked', () => {
        expect(renameIncompleteMessage([], false)).toBeNull()
    })
    it('names the addresses that were not kept', () => {
        expect(renameIncompleteMessage(['a@x.example', 'a@y.example'], false)).toBe(
            'The new email name is saved. These old addresses were not kept: a@x.example, a@y.example. Add them again in the address list.'
        )
    })
    it('says when Default From did not change', () => {
        expect(renameIncompleteMessage([], true)).toBe(
            'The new email name is saved. Default From did not change to the kept address. Choose it again.'
        )
    })
})

describe('emailNameSchema', () => {
    it('trims and lowercases', () => {
        expect(emailNameSchema.parse({ email_name: ' Anna ', keep_old: true }).email_name).toBe(
            'anna'
        )
    })
    it.each(['', 'an na', 'a'.repeat(65)])('refuses %j', value => {
        expect(emailNameSchema.safeParse({ email_name: value, keep_old: true }).success).toBe(false)
    })
})
