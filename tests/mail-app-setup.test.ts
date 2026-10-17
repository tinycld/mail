import { describe, expect, it } from 'vitest'
import {
    MAIL_APPS_PATH,
    profileUrl,
    setupAddress,
    setupPageUrl,
} from '~/tinycld/mail/settings/mail-app-setup'
import type { PersonAddress } from '~/tinycld/mail/settings/usePersonEmail'

function address(over: Partial<PersonAddress> & { id: string; localPart: string }): PersonAddress {
    return {
        domainId: 'd1',
        domainName: 'acme.example',
        source: 'manual',
        isPrimary: false,
        rank: 0,
        verified: true,
        groupName: null,
        created: '2026-01-01 00:00:00.000Z',
        ...over,
    }
}

describe('setupAddress', () => {
    const main = address({ id: 'a1', localPart: 'nathan', isPrimary: true })
    const second = address({ id: 'a2', localPart: 'nathan', domainName: 'acme.com', rank: 1 })
    const unverified = address({ id: 'a3', localPart: 'nate', verified: false, rank: -1 })

    it('uses the chosen Default From', () => {
        expect(setupAddress('a2', [main, second])).toBe('nathan@acme.com')
    })

    // Mirrors the server's defaultSendAddress: verified first, then domain
    // order, then the main address.
    it('picks as the server does when none is chosen', () => {
        expect(setupAddress('', [unverified, second, main])).toBe('nathan@acme.example')
    })

    it('prefers the main address when the rank ties', () => {
        const other = address({ id: 'a4', localPart: 'aaa' })
        expect(setupAddress('', [other, main])).toBe('nathan@acme.example')
    })

    it('breaks the last tie by the earlier creation time, not the local part', () => {
        const earlier = address({ id: 'a5', localPart: 'zed', created: '2026-01-01 00:00:00.000Z' })
        const later = address({ id: 'a6', localPart: 'amy', created: '2026-02-01 00:00:00.000Z' })
        expect(setupAddress('', [later, earlier])).toBe('zed@acme.example')
    })

    it('ignores a chosen id that is no longer an address of the inbox', () => {
        expect(setupAddress('gone', [second])).toBe('nathan@acme.com')
    })

    it('is null with no address', () => {
        expect(setupAddress('', [])).toBeNull()
    })
})

describe('urls', () => {
    it('builds the profile url with an encoded file token', () => {
        expect(profileUrl('https://acme.tinycld.org', 'a b+c')).toBe(
            'https://acme.tinycld.org/api/mail/mobileconfig?token=a%20b%2Bc'
        )
    })

    it('builds the set-up page url', () => {
        expect(MAIL_APPS_PATH).toBe('settings/account/mail/mail-apps')
        expect(setupPageUrl('https://acme.tinycld.org')).toBe(
            'https://acme.tinycld.org/a/settings/account/mail/mail-apps'
        )
    })
})
