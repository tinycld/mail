import type { VerificationDetails } from '@tinycld/app-generated/mail-api'

export interface VerificationChecks {
    domain: string
    mx_verified: boolean
    inbound_domain_verified: boolean
    spf_verified: boolean
    dkim_verified: boolean
    return_path_verified: boolean
}

export interface VerificationProblem {
    label: string
    detail: string
}

// What the last Verify found wrong, one line per failed check, in words for
// the person publishing the records. A Verify that fails says nothing on its
// own: the badge stays "Not verified", and without this they cannot tell a
// record that is missing from one that is wrong or not visible yet.
export function verificationProblems(
    checks: VerificationChecks,
    details: VerificationDetails | null | undefined
): VerificationProblem[] {
    if (!details) return []
    const problems: VerificationProblem[] = []
    const outbound = details.outbound
    if (outbound?.enrolled === 'no' || outbound?.enrolled === 'unknown') {
        problems.push({
            label: 'Mail provider',
            detail: providerDetail(outbound.enrolled, outbound.error),
        })
    }
    if (!checks.mx_verified) {
        problems.push({ label: 'MX record', detail: mxDetail(checks.domain, details) })
    }
    if (!checks.inbound_domain_verified) {
        problems.push({
            label: 'Receiving',
            detail:
                details.provider?.error ||
                'The mail provider is not set up to receive mail for this domain yet.',
        })
    }
    if (!checks.dkim_verified && outbound?.dkim_host) {
        problems.push({
            label: 'DKIM record',
            detail: `No matching TXT record found at ${outbound.dkim_host}.`,
        })
    }
    if (!checks.return_path_verified && outbound?.return_path_domain) {
        problems.push({
            label: 'Return-Path record',
            detail: `No CNAME record at ${outbound.return_path_domain} points to ${outbound.return_path_cname_value ?? 'the value shown'}.`,
        })
    }
    if (!checks.spf_verified) {
        problems.push({
            label: 'SPF',
            detail: 'The mail provider has not confirmed SPF for this domain yet.',
        })
    }
    return problems
}

function providerDetail(enrolled: string, error: string | undefined) {
    const base =
        enrolled === 'no'
            ? 'The mail provider does not have this domain.'
            : 'The mail provider could not be reached, so the sending records were not checked.'
    return error ? `${base} ${error}` : base
}

function mxDetail(domain: string, details: VerificationDetails) {
    const mx = details.mx
    const want = mx?.expected ? `10 ${mx.expected}` : 'the value shown'
    if (mx?.error) return `The MX lookup for ${domain} failed: ${mx.error}`
    if (mx?.actual?.length) {
        return `${domain} has MX ${mx.actual.join(', ')}, but it must be ${want}.`
    }
    return `No MX record found for ${domain}. Add one with the value ${want}.`
}
