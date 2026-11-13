import { describe, expect, it } from 'vitest'
import automation from '~/tinycld/mail/automation'

function triggerFields(id: string) {
    const trigger = automation.triggers.find(t => t.id === id)
    return (trigger?.fields ?? []).map(f => (typeof f === 'string' ? { key: f, label: f } : f))
}

describe('mail automation definitions', () => {
    // Inbound mail stores the address it arrived at in delivered_to. alias is
    // set only when mail is sent from a legacy alias, so a condition on it
    // never matched an arriving message. server/automation_conditions_test.go
    // proves the inbound side: delivered_to is set and a rule on it matches.
    it('reads the receiving address of an arriving message from delivered_to', () => {
        const fields = triggerFields('message-received')
        expect(fields).toContainEqual({ key: 'delivered_to', label: 'Received at address' })
        expect(fields.map(f => f.key)).not.toContain('alias')
    })

    it('offers sender facts on both inbound triggers', () => {
        for (const id of ['message-received', 'message-classified']) {
            const trigger = automation.triggers.find(t => t.id === id)
            const keys = (trigger && 'extraFields' in trigger ? trigger.extraFields : []).map(
                f => f.key
            )
            expect(keys).toContain('has_sent_to_sender')
            expect(keys).toContain('sender_in_contacts')
        }
    })

    it('fires message-classified on a new classifications row', () => {
        const trigger = automation.triggers.find(t => t.id === 'message-classified')
        expect(trigger).toMatchObject({ collection: 'classifications', on: 'create' })
        expect(triggerFields('message-classified').map(f => f.key)).toEqual([
            'label_name',
            'confidence',
            'stage',
            'action',
        ])
    })
})
