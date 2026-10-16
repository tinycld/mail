import { z } from '@tinycld/core/ui/form'
import { newRecordId } from 'pbtsdb/core'

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
    domainName: string
    source: string
    isPrimary: boolean
}

/**
 * The addresses a rename removes, each with a new id for its manual copy.
 * Rule addresses follow the name, so those with the old name go. The main
 * address goes when its name differs from the new one: a legacy main address
 * (john.smith for the name john) is not the old name but is still lost.
 * Manual rows survive a rename.
 */
export function keptAddresses(rows: KeptRow[], oldName: string, newName: string) {
    return rows
        .filter(row =>
            row.isPrimary
                ? row.localPart !== newName
                : row.source === 'rule' && row.localPart === oldName
        )
        .map(row => ({
            localPart: row.localPart,
            domainId: row.domainId,
            email: `${row.localPart}@${row.domainName}`,
            oldId: row.id,
            newId: newRecordId(),
            isPrimary: row.isPrimary,
        }))
}

/**
 * Pure: the new Default From when the current one is a kept rule address, else
 * null. The main address keeps its id through a rename, so a Default From on
 * it follows the new name with no change.
 */
export function defaultFromAfterRename(
    currentDefault: string,
    kept: { oldId: string; newId: string; isPrimary: boolean }[]
) {
    if (!currentDefault) return null
    return kept.find(k => k.oldId === currentDefault && !k.isPrimary)?.newId ?? null
}

/** Pure: the form error after a rename that saved the name but not every later step. */
export function renameIncompleteMessage(notKept: string[], defaultFromFailed: boolean) {
    const parts: string[] = []
    if (notKept.length > 0) {
        parts.push(
            `These old addresses were not kept: ${notKept.join(', ')}. Add them again in the address list.`
        )
    }
    if (defaultFromFailed) {
        parts.push('Default From did not change to the kept address. Choose it again.')
    }
    if (parts.length === 0) return null
    return ['The new email name is saved.', ...parts].join(' ')
}
