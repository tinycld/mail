import { describe, expect, it } from 'vitest'
import {
    actionText,
    confidenceText,
    stageText,
} from '../tinycld/mail/classifier/classification-display'

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
