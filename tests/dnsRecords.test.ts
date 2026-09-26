import type { OutboundCheckResult } from '@tinycld/app-generated/mail-api'
import { describe, expect, it } from 'vitest'
import { buildDnsRecords, hasUnpublishedDnsRecords } from '~/tinycld/mail/settings/DnsRecordsPanel'

describe('buildDnsRecords', () => {
    it('returns the DKIM and return-path records to publish', () => {
        const records = buildDnsRecords({
            spf: false,
            dkim: false,
            return_path: false,
            enrolled: 'yes',
            mx_verified: false,
            dkim_host: 'sel._domainkey.acme.com',
            dkim_text_value: 'k=rsa;p=X',
            return_path_domain: 'pm-bounces.acme.com',
            return_path_cname_value: 'pm.mtasv.net',
        })

        expect(records).toHaveLength(2)
        expect(records[0]).toMatchObject({
            type: 'TXT',
            host: 'sel._domainkey.acme.com',
            value: 'k=rsa;p=X',
            verified: false,
        })
        expect(records[1]).toMatchObject({
            type: 'CNAME',
            host: 'pm-bounces.acme.com',
            value: 'pm.mtasv.net',
            verified: false,
        })
    })

    // MX leads the list — an admin publishes DNS top-down, and inbound
    // routing comes before DKIM/return-path signing.
    it('leads with MX when an mx_host is available', () => {
        const records = buildDnsRecords({
            spf: false,
            dkim: false,
            return_path: false,
            enrolled: 'yes',
            mx_verified: false,
            mx_host: 'mx.example.com',
            dkim_host: 'pm._domainkey.example.com',
            dkim_text_value: 'k=rsa; p=abc',
        })

        expect(records.map(r => [r.label, r.type, r.host, r.value])).toEqual([
            ['MX', 'MX', '@', '10 mx.example.com'],
            ['DKIM', 'TXT', 'pm._domainkey.example.com', 'k=rsa; p=abc'],
        ])
    })

    // The MX row shows the last MX check, not a constant: an unpublished MX
    // must keep the panel open.
    it('marks the MX row from mx_verified', () => {
        const outbound = {
            spf: true,
            dkim: true,
            return_path: true,
            enrolled: 'yes',
            mx_host: 'mx.example.com',
        }
        expect(buildDnsRecords({ ...outbound, mx_verified: false })[0].verified).toBe(false)
        expect(hasUnpublishedDnsRecords({ ...outbound, mx_verified: false })).toBe(true)
        expect(buildDnsRecords({ ...outbound, mx_verified: true })[0].verified).toBe(true)
        expect(hasUnpublishedDnsRecords({ ...outbound, mx_verified: true })).toBe(false)
    })

    // Details stored before mx_verified existed lack it: unknown, not red.
    it('treats a missing mx_verified as unknown', () => {
        const outbound = {
            spf: true,
            dkim: true,
            return_path: true,
            enrolled: 'yes',
            mx_host: 'mx.example.com',
        } as OutboundCheckResult
        expect(buildDnsRecords(outbound)[0].verified).toBeNull()
        expect(hasUnpublishedDnsRecords(outbound)).toBe(false)
    })

    // No mx_host (e.g. SMTP in IMAP-fetch mode, which publishes no MX target
    // on our side) — the row must not render a "MX 10 " with nothing after it.
    it('omits the MX row when there is no mx_host', () => {
        const records = buildDnsRecords({
            spf: false,
            dkim: false,
            return_path: false,
            enrolled: 'yes',
            mx_verified: false,
            dkim_host: 'pm._domainkey.example.com',
            dkim_text_value: 'k=rsa; p=abc',
        })

        expect(records.map(r => r.label)).toEqual(['DKIM'])
    })

    it('marks a record verified once the provider confirms it', () => {
        const records = buildDnsRecords({
            spf: true,
            dkim: true,
            return_path: true,
            enrolled: 'yes',
            mx_verified: false,
            dkim_host: 'sel._domainkey.acme.com',
            dkim_text_value: 'k=rsa;p=X',
            return_path_domain: 'pm-bounces.acme.com',
            return_path_cname_value: 'pm.mtasv.net',
        })

        expect(records.every(r => r.verified)).toBe(true)
    })

    // Each row must read its OWN provider flag. Both cases above set every
    // flag to the same value, so a row reading a sibling's flag survives them:
    // with DKIM published but the return path not, the admin would see DKIM
    // green and return-path red — inverted — and go fix the wrong record.
    it('maps each record to its own provider flag', () => {
        const records = buildDnsRecords({
            spf: false,
            dkim: true,
            return_path: false,
            enrolled: 'yes',
            mx_verified: false,
            dkim_host: 'sel._domainkey.acme.com',
            dkim_text_value: 'k=rsa;p=X',
            return_path_domain: 'pm-bounces.acme.com',
            return_path_cname_value: 'pm.mtasv.net',
        })

        expect(records.map(r => [r.label, r.verified])).toEqual([
            ['DKIM', true],
            ['Return-Path', false],
        ])
    })

    // The mirror image, so neither row can be pinned by a constant.
    it('maps each record to its own provider flag when only the return path is verified', () => {
        const records = buildDnsRecords({
            spf: true,
            dkim: false,
            return_path: true,
            enrolled: 'yes',
            mx_verified: false,
            dkim_host: 'sel._domainkey.acme.com',
            dkim_text_value: 'k=rsa;p=X',
            return_path_domain: 'pm-bounces.acme.com',
            return_path_cname_value: 'pm.mtasv.net',
        })

        expect(records.map(r => [r.label, r.verified])).toEqual([
            ['DKIM', false],
            ['Return-Path', true],
        ])
    })

    // A self-hosted SMTP deployment enrolls nothing and has no records to
    // show: the panel must render nothing rather than empty rows.
    it('returns nothing when the provider supplied no records', () => {
        expect(
            buildDnsRecords({
                spf: false,
                dkim: false,
                return_path: false,
                enrolled: 'yes',
                mx_verified: false,
            })
        ).toEqual([])
    })

    it('returns nothing when there is no outbound result at all', () => {
        expect(buildDnsRecords(undefined)).toEqual([])
    })
})

// FIX 7: the panel must stay visible while there are still records to publish.
//
// It used to be gated on `!domain.verified`, but `verified` deliberately
// EXCLUDES DKIM and return-path — they are advisory. So the moment MX and the
// inbound domain went green, the badge flipped to Verified and the panel
// VANISHED, while the DKIM and Return-Path rows were still showing red and the
// host/value/copy UI needed to fix them was gone.
describe('hasUnpublishedDnsRecords', () => {
    const postmarkRecords = {
        dkim_host: 'sel._domainkey.acme.com',
        dkim_text_value: 'k=rsa;p=X',
        return_path_domain: 'pm-bounces.acme.com',
        return_path_cname_value: 'pm.mtasv.net',
    }

    it('stays visible when a record is still unpublished', () => {
        expect(
            hasUnpublishedDnsRecords({
                spf: true,
                dkim: true,
                return_path: false,
                enrolled: 'yes',
                mx_verified: false,
                ...postmarkRecords,
            })
        ).toBe(true)
    })

    it('stays visible when DKIM alone is unpublished', () => {
        expect(
            hasUnpublishedDnsRecords({
                spf: true,
                dkim: false,
                return_path: true,
                enrolled: 'yes',
                mx_verified: false,
                ...postmarkRecords,
            })
        ).toBe(true)
    })

    // The panel is not permanently pinned open: once every record it can show
    // is verified, it has nothing left to say.
    it('hides once every record is published', () => {
        expect(
            hasUnpublishedDnsRecords({
                spf: true,
                dkim: true,
                return_path: true,
                enrolled: 'yes',
                mx_verified: false,
                ...postmarkRecords,
            })
        ).toBe(false)
    })

    it('hides when the provider reported no records at all', () => {
        expect(
            hasUnpublishedDnsRecords({
                spf: false,
                dkim: false,
                return_path: false,
                enrolled: 'yes',
                mx_verified: false,
            })
        ).toBe(false)
        expect(hasUnpublishedDnsRecords(undefined)).toBe(false)
    })
})
