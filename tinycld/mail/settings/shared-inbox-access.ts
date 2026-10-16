/** One membership row of a person on a shared inbox; groupName '' is a direct row. */
export interface InboxMembershipRow {
    mailboxId: string
    label: string
    groupName: string
}

/**
 * Pure: one line per inbox. A direct row hides the groups, as the person has
 * the inbox without them; otherwise the lines name the groups that give it.
 */
export function inboxAccessLines(rows: InboxMembershipRow[]) {
    const byInbox = new Map<string, { label: string; isDirect: boolean; groups: string[] }>()
    for (const row of rows) {
        const entry = byInbox.get(row.mailboxId) ?? {
            label: row.label,
            isDirect: false,
            groups: [],
        }
        if (!row.groupName) entry.isDirect = true
        else if (!entry.groups.includes(row.groupName)) entry.groups.push(row.groupName)
        byInbox.set(row.mailboxId, entry)
    }
    return [...byInbox].map(([id, { label, isDirect, groups }]) => ({
        id,
        text: isDirect || groups.length === 0 ? label : `${label} · through ${groups.join(', ')}`,
    }))
}
