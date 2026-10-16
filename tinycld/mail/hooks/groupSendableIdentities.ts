import type { MailAddresses, MailDomains, MailIdentities, MailMailboxes } from '../types'

export interface SendableAddress {
    id: string
    email: string
    isPrimary: boolean
    domainRank: number
    domainVerified: boolean
    created: string
}

export interface SendableIdentity {
    mailboxId: string
    mailboxDisplayName: string
    isPersonal: boolean
    // Default first, then the server's default order.
    addresses: SendableAddress[]
    defaultAddressId: string
}

/** One row per (membership × address), from useSendableIdentities' query. */
export interface SendableIdentityRow {
    mailbox: MailMailboxes
    address: MailAddresses
    domain: MailDomains
    identity?: MailIdentities
}

/**
 * The order the server's defaultSendAddress (server/send_address.go) uses:
 * verified domains first, then the admin's domain order, then the inbox's main
 * address, then the oldest. Keep the two in step, or the picker shows a
 * different default from the one a send without from_address_id uses.
 */
export function compareSendOrder(a: SendableAddress, b: SendableAddress): number {
    if (a.domainVerified !== b.domainVerified) return a.domainVerified ? -1 : 1
    if (a.domainRank !== b.domainRank) return a.domainRank - b.domainRank
    if (a.isPrimary !== b.isPrimary) return a.isPrimary ? -1 : 1
    return a.created.localeCompare(b.created)
}

interface MailboxEntry {
    mailbox: MailMailboxes
    preferredId: string
    addresses: SendableAddress[]
}

function toSendableAddress(row: SendableIdentityRow): SendableAddress {
    return {
        id: row.address.id,
        email: `${row.address.local_part}@${row.domain.domain}`,
        isPrimary: row.address.is_primary,
        domainRank: row.domain.rank ?? 0,
        domainVerified: row.domain.verified,
        created: row.address.created,
    }
}

function toIdentity(entry: MailboxEntry): SendableIdentity {
    const sorted = [...entry.addresses].sort(compareSendOrder)
    const defaultAddress = sorted.find(a => a.id === entry.preferredId) ?? sorted[0]
    return {
        mailboxId: entry.mailbox.id,
        mailboxDisplayName: entry.mailbox.display_name || entry.mailbox.address,
        isPersonal: entry.mailbox.type === 'personal',
        addresses: [defaultAddress, ...sorted.filter(a => a.id !== defaultAddress.id)],
        defaultAddressId: defaultAddress.id,
    }
}

/**
 * Pure: group the rows into one identity per mailbox, personal first,
 * then shared by `created` ascending — the order pickers present them in.
 */
export function groupSendableIdentities(rows: SendableIdentityRow[]): SendableIdentity[] {
    const byMailbox = new Map<string, MailboxEntry>()
    for (const row of rows) {
        let entry = byMailbox.get(row.mailbox.id)
        if (!entry) {
            entry = {
                mailbox: row.mailbox,
                preferredId: row.identity?.default_from ?? '',
                addresses: [],
            }
            byMailbox.set(row.mailbox.id, entry)
        }
        if (entry.addresses.some(a => a.id === row.address.id)) continue
        entry.addresses.push(toSendableAddress(row))
    }

    const entries = Array.from(byMailbox.values())
    const personal = entries.filter(e => e.mailbox.type === 'personal')
    const shared = entries
        .filter(e => e.mailbox.type === 'shared')
        .sort((a, b) => a.mailbox.created.localeCompare(b.mailbox.created))
    return [...personal, ...shared].map(toIdentity)
}
