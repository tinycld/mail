export interface DomainChecks {
    verified: boolean
    spf_verified: boolean
    dkim_verified: boolean
    return_path_verified: boolean
}

// A domain is ready when it can both receive and send. The row's own
// `verified` flag covers receiving only (MX + inbound); the send gate also
// refuses a domain without SPF, DKIM and Return-Path, so a domain shown as
// verified without them would fail its first send.
export function isDomainReady(d: DomainChecks): boolean {
    return d.verified && d.spf_verified && d.dkim_verified && d.return_path_verified
}
