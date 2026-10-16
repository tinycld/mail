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

/** Pure: the notice sentence, or null when nothing is lost. personName null: the group is deleted. */
export function removalNoticeText({
    personName,
    addresses,
}: {
    personName: string | null
    addresses: string[]
}) {
    const n = addresses.length
    if (n === 0) return null
    const who = personName ? `${personName} loses` : 'Members lose'
    if (n === 1) return `${who} the address ${addresses[0]}.`
    if (personName) return `${who} these addresses: ${shortList(addresses)}.`
    return `${who} ${n} addresses: ${shortList(addresses)}.`
}
