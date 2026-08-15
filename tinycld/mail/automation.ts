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
        {
            // A delivery failure is a different event from "a message
            // arrives", even though both are mail_messages rows: the message
            // is one YOU sent, and the interesting fields are the failure. A
            // Go filter (server/automation.go) gates it to the two statuses
            // that actually mean failure, so an ordinary send transitioning
            // sending → sent never fires this.
            id: 'message-bounced',
            label: 'A message bounces',
            collection: 'mail_messages',
            on: 'update',
            watch: ['delivery_status'],
            fields: [
                'subject',
                { key: 'bounce_reason', label: 'Reason' },
                { key: 'delivery_status', label: 'Delivery status' },
                { key: 'sender_email', label: 'Sent from' },
            ],
        },
    ],
    // All four actions are native. Folder and read state live per-user on
    // mail_thread_state, not on the mail_messages row the trigger fires for,
    // so a record-op (which can only target the trigger record) cannot
    // express them; sending runs through the provider + persistence path.
    //
    // Scope: an org rule acts for every member of the message's mailbox, a
    // personal rule only for its owner. See server/automation.go.
    actions: [
        {
            id: 'move-to-folder',
            label: 'Move to folder',
            kind: 'native',
            params: [
                {
                    key: 'folder',
                    type: 'select',
                    label: 'Folder',
                    // Deliberately narrower than mail_thread_state.folder,
                    // which also allows sent/drafts — neither is a sensible
                    // destination for a rule.
                    options: ['archive', 'trash', 'spam', 'inbox'],
                },
            ],
        },
        {
            id: 'mark-as-read',
            label: 'Mark as read',
            kind: 'native',
        },
        {
            id: 'star-message',
            label: 'Star the message',
            kind: 'native',
        },
        {
            id: 'forward-message',
            label: 'Forward the message',
            kind: 'native',
            params: [{ key: 'to', type: 'text', label: 'Forward to' }],
        },
        {
            id: 'send-message',
            label: 'Send a message',
            kind: 'native',
            params: [
                { key: 'to', type: 'text', label: 'To' },
                { key: 'subject', type: 'text', label: 'Subject' },
                { key: 'body', type: 'text', label: 'Body' },
            ],
        },
    ],
} satisfies AutomationDefinitions<MailSchema>

export default automation
