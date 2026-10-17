import { appHref } from '@tinycld/core/lib/org-routes'
import type { PersonAddress } from './usePersonEmail'

// The ports every mail app answer gives (server/client_discovery.go).
export const IMAP_PORT = 993
export const SMTP_PORT = 465

/** Settings → Account → Mail apps, under core's per-user panel route. */
export const MAIL_APPS_PATH = 'settings/account/mail/mail-apps'

// The order the server picks a sender in when none is chosen
// (send_address.go defaultSendAddress): verified, domain order, main address.
// Both break a last tie by creation time (PocketBase dates sort as text).
function compareForDefault(a: PersonAddress, b: PersonAddress) {
    if (a.verified !== b.verified) return a.verified ? -1 : 1
    if (a.rank !== b.rank) return a.rank - b.rank
    if (a.isPrimary !== b.isPrimary) return a.isPrimary ? -1 : 1
    return a.created < b.created ? -1 : a.created > b.created ? 1 : 0
}

/** The address the set-up page and the profile use: the person's Default From. */
export function setupAddress(defaultFrom: string, addresses: PersonAddress[]): string | null {
    const pick =
        addresses.find(a => a.id === defaultFrom) ?? [...addresses].sort(compareForDefault)[0]
    return pick ? `${pick.localPart}@${pick.domainName}` : null
}

export function profileUrl(serverAddress: string, profileToken: string): string {
    return `${serverAddress}/api/mail/mobileconfig?token=${encodeURIComponent(profileToken)}`
}

export function setupPageUrl(serverAddress: string): string {
    return `${serverAddress}${appHref(MAIL_APPS_PATH)}`
}
