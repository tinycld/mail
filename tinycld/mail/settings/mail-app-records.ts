import type { DnsRecord } from './DnsRecordsPanel'
import { IMAP_PORT, SMTP_PORT } from './mail-app-setup'

// Outlook follows _autodiscover._tcp to an HTTPS POST.
const HTTPS_PORT = 443

function srv(label: string, host: string, port: number, target: string): DnsRecord {
    return { label, type: 'SRV', host, value: `0 1 ${port} ${target}`, verified: null }
}

function cname(label: string, name: string, domain: string, target: string): DnsRecord | null {
    // A web host of mail.<domain> already is the name: a CNAME to itself is an error.
    if (`${name}.${domain}` === target) return null
    return {
        label,
        type: 'CNAME',
        host: name,
        value: target,
        verified: null,
        note: `Works only when this server has a TLS certificate for ${name}.${domain}.`,
    }
}

/**
 * The optional records that let mail apps find their settings (spec 9.3).
 * The SRV records come first: they point at the mail host itself, so they
 * work with the certificate it already has.
 */
export function buildMailAppRecords(domain: string, mailHost: string): DnsRecord[] {
    if (!mailHost) return []
    const records = [
        srv('IMAP server', '_imaps._tcp', IMAP_PORT, mailHost),
        srv('SMTP server', '_submissions._tcp', SMTP_PORT, mailHost),
        srv('Outlook', '_autodiscover._tcp', HTTPS_PORT, mailHost),
        cname('Thunderbird', 'autoconfig', domain, mailHost),
        cname('Apple Mail', 'mail', domain, mailHost),
    ]
    return records.filter(record => record !== null)
}
