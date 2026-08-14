import type { AutomationDefinitions } from '@tinycld/core/lib/automation/types'
import type { MailSchema } from './types'

// NOTE: mail_messages has no user FK (mailboxes are shared) — personal-rule
// owner resolution for this trigger is defined in the engine phase. No
// ownerField is declared here on purpose.
const automation = {
    triggers: [
        {
            id: 'message-received',
            label: 'A message arrives',
            collection: 'mail_messages',
            on: 'create',
            fields: [
                'subject',
                { key: 'sender_email', label: 'Sender' },
                { key: 'sender_name', label: 'Sender name' },
                'has_attachments',
                { key: 'alias', label: 'Received via alias' },
            ],
        },
    ],
} satisfies AutomationDefinitions<MailSchema>

export default automation
