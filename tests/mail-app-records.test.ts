import { describe, expect, it } from 'vitest'
import { buildMailAppRecords } from '~/tinycld/mail/settings/mail-app-records'

describe('buildMailAppRecords', () => {
    it('lists the SRV records first, then the CNAMEs with a certificate note', () => {
        const records = buildMailAppRecords('acme.com', 'acme.tinycld.org')
        expect(records.map(r => [r.type, r.host, r.value])).toEqual([
            ['SRV', '_imaps._tcp', '0 1 993 acme.tinycld.org'],
            ['SRV', '_submissions._tcp', '0 1 465 acme.tinycld.org'],
            ['SRV', '_autodiscover._tcp', '0 1 443 acme.tinycld.org'],
            ['CNAME', 'autoconfig', 'acme.tinycld.org'],
            ['CNAME', 'mail', 'acme.tinycld.org'],
        ])
        expect(records.every(r => r.verified === null)).toBe(true)
        expect(records.find(r => r.host === 'mail')?.note).toBe(
            'Works only when this server has a TLS certificate for mail.acme.com.'
        )
        expect(records.find(r => r.host === 'autoconfig')?.note).toBe(
            'Works only when this server has a TLS certificate for autoconfig.acme.com.'
        )
        expect(records.find(r => r.host === '_imaps._tcp')?.note).toBeUndefined()
    })

    it('leaves out a CNAME that would point at itself', () => {
        const hosts = buildMailAppRecords('acme.com', 'mail.acme.com').map(r => r.host)
        expect(hosts).not.toContain('mail')
        expect(hosts).toContain('autoconfig')
        expect(
            buildMailAppRecords('acme.com', 'autoconfig.acme.com').map(r => r.host)
        ).not.toContain('autoconfig')
    })

    it('is empty without a mail host', () => {
        expect(buildMailAppRecords('acme.com', '')).toEqual([])
    })
})
