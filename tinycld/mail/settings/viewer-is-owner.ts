/** Pure: whether the viewer holds the owner role on this inbox; only owners may write to it. */
export function viewerIsOwner(members: { isYou: boolean; role: string }[]) {
    return members.some(m => m.isYou && m.role === 'owner')
}
