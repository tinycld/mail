import { shortList } from './coverage-preview'

/** A rule address the removed group gave, with its owner and the domain's groups. */
export interface GroupAddressRow {
    email: string
    userId: string
    coverageGroups: string[]
}

/**
 * Pure: the addresses that stop receiving mail, sorted. A person in another
 * of the domain's groups keeps the address: the server moves its via_group to
 * that group (server/rule_addresses.go). The removed group itself never
 * counts, so this serves a member's removal and the group's deletion alike.
 */
export function addressesLost(
    rows: GroupAddressRow[],
    memberships: Map<string, string[]>,
    removedGroupId: string
) {
    const lost = new Set<string>()
    for (const row of rows) {
        const otherGroups = (memberships.get(row.userId) ?? []).filter(g => g !== removedGroupId)
        const isKept = otherGroups.some(g => row.coverageGroups.includes(g))
        if (!isKept) lost.add(row.email)
    }
    return [...lost].sort()
}

/** One row of a person on an inbox the removed group grants; viaGroup '' is a direct row. */
export interface InboxAccessRow {
    mailboxId: string
    email: string
    userId: string
    viaGroup: string
}

/**
 * Pure: the inboxes that stop being open to someone, sorted. The rows are
 * every row of the people on the inboxes the group grants. An inbox is kept
 * while the person has a row besides the removed group's derived row.
 */
export function inboxesLost(rows: InboxAccessRow[], removedGroupId: string) {
    const kept = new Set<string>()
    const derived = new Map<string, string>()
    for (const row of rows) {
        const key = `${row.mailboxId}\u0000${row.userId}`
        if (row.viaGroup === removedGroupId) derived.set(key, row.email)
        else kept.add(key)
    }
    const lost = new Set<string>()
    for (const [key, email] of derived) {
        if (!kept.has(key)) lost.add(email)
    }
    return [...lost].sort()
}

function addressesText(who: string, personName: string | null, addresses: string[]) {
    const n = addresses.length
    if (n === 1) return `${who} the address ${addresses[0]}.`
    if (personName) return `${who} these addresses: ${shortList(addresses)}.`
    return `${who} ${n} addresses: ${shortList(addresses)}.`
}

function inboxesText(who: string, personName: string | null, inboxes: string[]) {
    if (inboxes.length === 1) return `${who} access to ${inboxes[0]}.`
    if (personName) return `${who} access to these shared inboxes: ${shortList(inboxes)}.`
    return `${who} access to ${inboxes.length} shared inboxes: ${shortList(inboxes)}.`
}

function groupAddressesText(personName: string | null, groupAddresses: string[]) {
    const one = groupAddresses.length === 1
    if (personName) {
        const target = one ? groupAddresses[0] : `these addresses: ${shortList(groupAddresses)}`
        return `${personName} stops getting mail sent to ${target}.`
    }
    if (one) return `The address ${groupAddresses[0]} is deleted.`
    return `These addresses are deleted: ${shortList(groupAddresses)}.`
}

function lossText(personName: string | null, addresses: string[], inboxes: string[]) {
    const who = personName ?? 'Members'
    const loses = personName ? 'loses' : 'lose'
    if (addresses.length === 0 && inboxes.length === 0) return null
    if (inboxes.length === 0) return addressesText(`${who} ${loses}`, personName, addresses)
    if (addresses.length === 0) return inboxesText(`${who} ${loses}`, personName, inboxes)
    const first = addressesText(`${who} ${loses}`, personName, addresses)
    return `${first} ${inboxesText(`${who} also ${loses}`, personName, inboxes)}`
}

/**
 * Pure: the notice text, or null when nothing is lost. personName null: the
 * group is deleted. Addresses and inboxes get one sentence each: a long list
 * ends in "and 2 more", so one sentence for both would have two "and"s. The
 * group's own addresses come last, as a sentence of their own.
 */
export function removalNoticeText({
    personName,
    addresses,
    inboxes = [],
    groupAddresses = [],
}: {
    personName: string | null
    addresses: string[]
    inboxes?: string[]
    groupAddresses?: string[]
}) {
    const loss = lossText(personName, addresses, inboxes)
    if (groupAddresses.length === 0) return loss
    const own = groupAddressesText(personName, groupAddresses)
    return loss ? `${loss} ${own}` : own
}
