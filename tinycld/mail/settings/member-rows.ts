export type MemberRole = 'owner' | 'member'

export interface MemberRow {
    id: string
    userId: string
    userName: string
    userEmail: string
    role: MemberRole
    isYou: boolean
    /**
     * The group whose grant gave this row, or null for a direct row. This,
     * not the group name, decides whether the access comes from a group: the
     * group row can be missing (not loaded, or the viewer cannot read groups).
     */
    groupId: string | null
    /** Names of the groups that give this access, for display only. */
    groupNames: string[]
}

/**
 * Folds the membership rows of one inbox to one row per person. A direct row
 * (no group) comes first and wins, so remove and role changes act on it; a
 * person with only group-derived rows keeps the first derived row's id and
 * lists every group that gives access.
 */
export function foldMembers(rows: MemberRow[]): MemberRow[] {
    const byUser = new Map<string, MemberRow[]>()
    for (const row of rows) {
        const list = byUser.get(row.userId) ?? []
        list.push(row)
        byUser.set(row.userId, list)
    }
    return [...byUser.values()].map(list => {
        const ordered = [...list].sort(
            (a, b) => Number(a.groupId !== null) - Number(b.groupId !== null)
        )
        const [first] = ordered
        const role = ordered.some(r => r.role === 'owner') ? 'owner' : 'member'
        if (first.groupId === null) return { ...first, role }
        const groupNames = [...new Set(ordered.flatMap(r => r.groupNames))]
        return { ...first, role, groupNames }
    })
}

/** The note under a person who has access through groups. */
export function viaGroupLabel(groupNames: string[]): string {
    if (groupNames.length === 0) return 'Through a group'
    if (groupNames.length === 1) return `Through group ${groupNames[0]}`
    return `Through groups ${groupNames.join(', ')}`
}
