import { describe, expect, it } from 'vitest'
import { aliasSchemaFor } from '~/tinycld/mail/settings/aliasSchema'

const schema = aliasSchemaFor({
    mailboxAddresses: new Set(['support']),
    aliasAddresses: new Set(['help']),
})

const messageFor = (address: string) =>
    schema.safeParse({ address }).error?.issues.map(i => i.message)

describe('aliasSchemaFor', () => {
    it('trims and lowercases the address', () => {
        expect(schema.parse({ address: '  Sales.Team ' })).toEqual({ address: 'sales.team' })
    })

    it('rejects a blank or whitespace-only address', () => {
        expect(messageFor('   ')).toContain('Address is required')
    })

    it('rejects characters outside the local-part set', () => {
        expect(messageFor('a b')).toContain(
            'Only lowercase letters, numbers, dots, hyphens, underscores'
        )
    })

    it('rejects an address taken by a primary mailbox on the domain', () => {
        expect(messageFor('Support')).toContain(
            'Address is already a primary mailbox on this domain'
        )
    })

    it('rejects an address taken by another alias on the domain', () => {
        expect(messageFor(' help')).toContain('Address is already an alias on this domain')
    })
})
