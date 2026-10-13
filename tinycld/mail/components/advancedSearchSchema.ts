import { z } from '@tinycld/core/ui/form'

const ISO_DATE = /^\d{4}-\d{2}-\d{2}$/

// The regex alone accepts 2026-02-30; a round trip through Date rejects
// any day the calendar does not have.
export function isCalendarDate(value: string) {
    if (!ISO_DATE.test(value)) return false
    const parsed = new Date(`${value}T00:00:00Z`)
    return !Number.isNaN(parsed.getTime()) && parsed.toISOString().startsWith(value)
}

export const advancedSearchSchema = z
    .object({
        from: z.string().trim(),
        to: z.string().trim(),
        subject: z.string().trim(),
        hasWords: z.string().trim(),
        dateWithin: z.enum(['', '1d', '3d', '1w', '2w', '1m', '2m', '6m', '1y']),
        dateAnchor: z.string().trim(),
        folder: z.enum(['all', 'inbox', 'sent', 'drafts', 'trash', 'spam', 'starred']),
        hasAttachment: z.boolean(),
    })
    // The anchor only matters with a "Date within" range; an unparseable one
    // used to drop the date filter silently at query time.
    .refine(data => !data.dateWithin || isCalendarDate(data.dateAnchor), {
        message: 'Date must be a real date in YYYY-MM-DD format',
        path: ['dateAnchor'],
    })

export type AdvancedSearchFormValues = z.infer<typeof advancedSearchSchema>
