export interface AddressRowData {
    id: string
    localPart: string
    domainName: string
    source: string
    isPrimary: boolean
    rank: number
}

export interface AddressItem {
    id: string
    email: string
    badge: string | null
    canRemove: boolean
}

function badgeFor(row: AddressRowData) {
    if (row.isPrimary) return 'Main address'
    if (row.source === 'rule') return 'From the domain rule'
    return null
}

// Main address first, then by the admin's domain order, then alphabetical.
function compare(a: AddressRowData, b: AddressRowData) {
    if (a.isPrimary !== b.isPrimary) return a.isPrimary ? -1 : 1
    if (a.rank !== b.rank) return a.rank - b.rank
    return a.localPart.localeCompare(b.localPart)
}

/** Pure: the panel's rows. Only an address an admin added can be removed here. */
export function toAddressItems(rows: AddressRowData[]): AddressItem[] {
    return [...rows].sort(compare).map(row => ({
        id: row.id,
        email: `${row.localPart}@${row.domainName}`,
        badge: badgeFor(row),
        canRemove: !row.isPrimary && row.source === 'manual',
    }))
}
