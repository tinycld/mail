export type MailboxType = 'shared' | 'personal'

export interface MailboxListItem {
    id: string
    address: string
    domainName: string
    displayName: string
    type: string
    memberCount: number
    otherAddressCount: number
    memberNames: string[]
    memberEmails: string[]
    otherAddresses: string[]
}

export interface FilterInput {
    query: string
}

export interface GroupedMailboxes {
    shared: MailboxListItem[]
    totals: { shared: number }
}

function matchesQuery(item: MailboxListItem, q: string): boolean {
    if (!q) return true
    const needle = q.toLowerCase().trim()
    if (!needle) return true
    if (item.address.toLowerCase().includes(needle)) return true
    if (item.displayName.toLowerCase().includes(needle)) return true
    for (const n of item.memberNames) {
        if (n.toLowerCase().includes(needle)) return true
    }
    for (const e of item.memberEmails) {
        if (e.toLowerCase().includes(needle)) return true
    }
    for (const a of item.otherAddresses) {
        if (a.toLowerCase().includes(needle)) return true
    }
    return false
}

function byAddress(a: MailboxListItem, b: MailboxListItem): number {
    return a.address.localeCompare(b.address)
}

export function filterAndGroupMailboxes(
    items: MailboxListItem[],
    input: FilterInput
): GroupedMailboxes {
    const sharedItems = items.filter(i => i.type === 'shared')
    const shared = sharedItems.filter(i => matchesQuery(i, input.query)).sort(byAddress)

    return { shared, totals: { shared: sharedItems.length } }
}
