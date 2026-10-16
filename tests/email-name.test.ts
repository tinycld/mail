import { describe, expect, it } from 'vitest'
import {
    defaultFromAfterRename,
    emailNameSchema,
    keptAddresses,
} from '~/tinycld/mail/settings/email-name'

const rows = [
    { id: 'a1', localPart: 'nathan', domainId: 'd1', source: 'rule' },
    { id: 'a2', localPart: 'nathan', domainId: 'd2', source: 'rule' },
    { id: 'a3', localPart: 'nathan', domainId: 'd3', source: 'manual' },
    { id: 'a4', localPart: 'sales', domainId: 'd1', source: 'rule' },
]

describe('keptAddresses', () => {
    it('keeps only the rule rows with the old local part', () => {
        expect(keptAddresses(rows, 'nathan')).toEqual([
            { localPart: 'nathan', domainId: 'd1', oldId: 'a1' },
            { localPart: 'nathan', domainId: 'd2', oldId: 'a2' },
        ])
    })
})

describe('defaultFromAfterRename', () => {
    const kept = [{ oldId: 'a1' }, { oldId: 'a2' }]
    const newIds = new Map([
        ['a1', 'n1'],
        ['a2', 'n2'],
    ])
    it('returns the new id when the default was a kept rule row', () => {
        expect(defaultFromAfterRename('a2', kept, newIds)).toBe('n2')
    })
    it('returns null when the default is a manual row', () => {
        expect(defaultFromAfterRename('a3', kept, newIds)).toBeNull()
    })
    it('returns null when the default is empty', () => {
        expect(defaultFromAfterRename('', kept, newIds)).toBeNull()
    })
    it('returns null when keep-old is off', () => {
        expect(defaultFromAfterRename('a1', [], new Map())).toBeNull()
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
