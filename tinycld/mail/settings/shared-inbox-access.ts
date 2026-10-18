/**
 * One membership row of a person on a shared inbox. groupId '' is a direct
 * row; groupName is for display only, as the group row may not be loaded.
 */
export interface InboxMembershipRow {
    mailboxId: string
    label: string
    groupId: string
    groupName: string
}

/**
 * Pure: one line per inbox. A direct row hides the groups, as the person has
 * the inbox without them; otherwise the lines name the groups that give it.
 */
export function inboxAccessLines(rows: InboxMembershipRow[]) {
    const byInbox = new Map<
        string,
        { label: string; isDirect: boolean; groups: Map<string, string> }
    >()
    for (const row of rows) {
        const entry = byInbox.get(row.mailboxId) ?? {
            label: row.label,
            isDirect: false,
            groups: new Map<string, string>(),
        }
        if (!row.groupId) entry.isDirect = true
        else entry.groups.set(row.groupId, row.groupName || 'a group')
        byInbox.set(row.mailboxId, entry)
    }
    return [...byInbox].map(([id, { label, isDirect, groups }]) => ({
        id,
        text:
            isDirect || groups.size === 0
                ? label
                : `${label} · through ${[...groups.values()].join(', ')}`,
    }))
}
