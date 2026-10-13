import { describe, expect, it } from 'vitest'
import {
    advancedSearchSchema,
    isCalendarDate,
} from '~/tinycld/mail/components/advancedSearchSchema'

const base = {
    from: '',
    to: '',
    subject: '',
    hasWords: '',
    dateWithin: '' as const,
    dateAnchor: '2026-10-13',
    folder: 'all' as const,
    hasAttachment: false,
}

describe('isCalendarDate', () => {
    it('accepts a real YYYY-MM-DD date', () => {
        expect(isCalendarDate('2024-02-29')).toBe(true)
    })

    it('rejects a day the calendar does not have', () => {
        expect(isCalendarDate('2026-02-30')).toBe(false)
    })

    it('rejects other formats', () => {
        expect(isCalendarDate('10/13/2026')).toBe(false)
        expect(isCalendarDate('')).toBe(false)
    })
})

describe('advancedSearchSchema', () => {
    it('ignores the anchor when no date range is chosen', () => {
        expect(advancedSearchSchema.safeParse({ ...base, dateAnchor: 'soon' }).success).toBe(true)
    })

    it('requires a real anchor date when a date range is chosen', () => {
        const result = advancedSearchSchema.safeParse({
            ...base,
            dateWithin: '1w',
            dateAnchor: 'last tuesday',
        })
        expect(result.error?.issues[0]).toMatchObject({
            path: ['dateAnchor'],
            message: 'Date must be a real date in YYYY-MM-DD format',
        })
    })

    it('trims text filters so whitespace alone is no filter', () => {
        const parsed = advancedSearchSchema.parse({ ...base, from: '  ', subject: ' invoice ' })
        expect(parsed.from).toBe('')
        expect(parsed.subject).toBe('invoice')
    })
})
