import { describe, expect, it } from 'vitest'
import { buildDnsRecords, hasUnpublishedDnsRecords } from '~/tinycld/mail/settings/DnsRecordsPanel'

describe('buildDnsRecords', () => {
    it('returns the DKIM and return-path records to publish', () => {
        const records = buildDnsRecords({
            spf: false,
            dkim: false,
            return_path: false,
            enrolled: 'yes',
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

    it('marks a record verified once the provider confirms it', () => {
        const records = buildDnsRecords({
            spf: true,
            dkim: true,
            return_path: true,
            enrolled: 'yes',
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
            buildDnsRecords({ spf: false, dkim: false, return_path: false, enrolled: 'yes' })
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
            })
        ).toBe(false)
        expect(hasUnpublishedDnsRecords(undefined)).toBe(false)
    })
})
