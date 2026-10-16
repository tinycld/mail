import { z } from '@tinycld/core/ui/form'

export const emailNameSchema = z.object({
    email_name: z
        .string()
        .trim()
        .toLowerCase()
        .min(1, 'Email name is required')
        .max(64, 'Use at most 64 characters')
        .regex(/^[a-z0-9._-]+$/, 'Use only a–z, 0–9, dot, dash and underscore'),
    keep_old: z.boolean(),
})

export type EmailNameValues = z.infer<typeof emailNameSchema>

interface KeptRow {
    id: string
    localPart: string
    domainId: string
    source: string
}

/** Pure: the rule addresses a rename removes. Manual rows survive a rename. */
export function keptAddresses(rows: KeptRow[], oldName: string) {
    return rows
        .filter(row => row.source === 'rule' && row.localPart === oldName)
        .map(row => ({ localPart: row.localPart, domainId: row.domainId, oldId: row.id }))
}

/** Pure: the new Default From when the current one is a kept address, else null. */
export function defaultFromAfterRename(
    currentDefault: string,
    kept: { oldId: string }[],
    newIds: Map<string, string>
) {
    if (!currentDefault || !kept.some(k => k.oldId === currentDefault)) return null
    return newIds.get(currentDefault) ?? null
}
