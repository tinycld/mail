import { describe, expect, it } from 'vitest'
import { domainRemovalWarning } from '~/tinycld/mail/settings/domain-removal'

describe('domainRemovalWarning', () => {
    it('says mail stops when the domain has no mailboxes', () => {
        expect(domainRemovalWarning('example.com', 0)).toBe(
            'Removing example.com stops mail for it. This cannot be undone.'
        )
    })

    it('names the mailbox count that is deleted with the domain', () => {
        expect(domainRemovalWarning('example.com', 3)).toBe(
            'Removing example.com also deletes its 3 mailboxes and all their mail. This cannot be undone.'
        )
    })

    it('uses the singular for one mailbox', () => {
        expect(domainRemovalWarning('example.com', 1)).toBe(
            'Removing example.com also deletes its 1 mailbox and all its mail. This cannot be undone.'
        )
    })
})
