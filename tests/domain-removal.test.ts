import { describe, expect, it } from 'vitest'
import { domainRemovalWarning } from '~/tinycld/mail/settings/domain-removal'

describe('domainRemovalWarning', () => {
    it('says the mailboxes and their mail go with the domain', () => {
        expect(domainRemovalWarning('example.com')).toBe(
            'Removing example.com also deletes every mailbox on it and all their mail. This cannot be undone.'
        )
    })
})
