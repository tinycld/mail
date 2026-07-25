import type { MailThreadState } from '../types'

export function mergeSharedFolderStates(
    states: MailThreadState[],
    coMemberUserIds: string[]
): MailThreadState[] {
    const coMemberSet = new Set(coMemberUserIds)
    const seen = new Set<string>()
    const merged: MailThreadState[] = []
    for (const s of states) {
        if (!coMemberSet.has(s.user)) continue
        if (seen.has(s.thread)) continue
        seen.add(s.thread)
        merged.push(s)
    }
    return merged
}
