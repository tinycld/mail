export type MemberRole = 'owner' | 'member'

export interface MemberRow {
    id: string
    userId: string
    userName: string
    userEmail: string
    role: MemberRole
    isYou: boolean
    /** Names of the groups that give this person access, when no direct row does. */
    viaGroupName: string | null
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
            (a, b) => Number(a.viaGroupName !== null) - Number(b.viaGroupName !== null)
        )
        const [first] = ordered
        const role = ordered.some(r => r.role === 'owner') ? 'owner' : 'member'
        if (first.viaGroupName === null) return { ...first, role }
        const names = [...new Set(ordered.map(r => r.viaGroupName))]
        return { ...first, role, viaGroupName: names.join(', ') }
    })
}
