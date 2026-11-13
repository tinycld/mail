import { z } from '@tinycld/core/ui/form'

export const DEFAULT_ARCHIVE_THRESHOLD = 0.9

// The field stays a string (the text input's native type): z.coerce.number
// would turn a blank value into 0 and slip past the 0.5 floor check's intent.
export const archiveThresholdSchema = z.object({
    value: z
        .string()
        .trim()
        .min(1, 'Enter a value')
        .refine(v => Number.isFinite(Number(v)), 'Enter a number')
        .refine(v => Number(v) >= 0.5, 'Use 0.5 or more')
        .refine(v => Number(v) <= 1, 'Use 1 or less'),
})

export type ArchiveThresholdValues = z.infer<typeof archiveThresholdSchema>
