interface GroupDomainRow {
    id: string
    domain: string
    coverageGroups: string[]
    rank: number
}

/** Pure: a TanStack DB filter cannot test a multi-relation for one value. */
export function domainsForGroup<T extends GroupDomainRow>(rows: T[], groupId: string): T[] {
    return rows.filter(r => r.coverageGroups.includes(groupId)).sort((a, b) => a.rank - b.rank)
}
