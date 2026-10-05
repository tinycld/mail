import { describe, expect, it } from 'vitest'
import { prettifyFolderKey, stripHtmlTags } from '~/tinycld/mail/hooks/mailListHelpers'

describe('prettifyFolderKey', () => {
    it('returns the canonical title when one is registered', () => {
        expect(prettifyFolderKey('all-inboxes')).toBe('All Inboxes')
    })

    it('title-cases unknown single-word keys', () => {
        expect(prettifyFolderKey('inbox')).toBe('Inbox')
        expect(prettifyFolderKey('sent')).toBe('Sent')
    })

    it('title-cases each segment of an unregistered hyphenated key', () => {
        expect(prettifyFolderKey('shared-with-me')).toBe('Shared With Me')
    })

    it('handles an empty key without throwing', () => {
        expect(prettifyFolderKey('')).toBe('')
    })
})

describe('stripHtmlTags', () => {
    it('removes simple tags', () => {
        expect(stripHtmlTags('hello <b>world</b>')).toBe('hello world')
    })

    it('removes attributed tags', () => {
        expect(stripHtmlTags('<a href="x">link</a>')).toBe('link')
    })
})
