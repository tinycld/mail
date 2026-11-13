import { describe, expect, it } from 'vitest'
import {
    actionText,
    confidenceText,
    stageText,
} from '../tinycld/mail/classifier/classification-display'
import { archiveThresholdSchema } from '../tinycld/mail/settings/archive-threshold-schema'

const base = { confidence: 0.92, stage: 'metadata', action: 'label', preview: false, error: '' }

describe('classification display', () => {
    it('words the action', () => {
        expect(actionText(base)).toBe('Labelled')
        expect(actionText({ ...base, action: 'label_archive' })).toBe('Labelled and archived')
        expect(actionText({ ...base, preview: true })).toBe('Not changed (preview)')
    })

    it('words the error', () => {
        expect(actionText({ ...base, error: 'budget_exceeded' })).toBe('Budget reached')
        expect(actionText({ ...base, error: 'auth_failed' })).toBe('Not classified')
    })

    it('formats confidence and stage', () => {
        expect(confidenceText(0.924)).toBe('92%')
        expect(confidenceText(null)).toBe('0%')
        expect(confidenceText(0, 'budget_exceeded')).toBe('')
        expect(stageText('full')).toBe('Full text')
        expect(stageText('metadata')).toBe('Metadata')
    })
})

describe('archive threshold schema', () => {
    const ok = (value: string) => archiveThresholdSchema.safeParse({ value }).success

    it('accepts 0.5 to 1', () => {
        expect(ok('0.5')).toBe(true)
        expect(ok('0.9')).toBe(true)
        expect(ok('1')).toBe(true)
    })

    it('rejects blank, text and out-of-range values', () => {
        expect(ok('')).toBe(false)
        expect(ok('  ')).toBe(false)
        expect(ok('abc')).toBe(false)
        expect(ok('0.4')).toBe(false)
        expect(ok('1.1')).toBe(false)
    })
})
