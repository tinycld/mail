/// <reference path="../../../server/pb_data/types.d.ts" />

// Separates the two ids a sent message has.
//
// message_id held whatever the outbound provider returned. For Postmark that
// is its own tracking id, not the Message-ID header recipients see, so a reply
// from outside never matched its thread and an IMAP client's APPENDed copy of
// the same message never matched the stored one (Apple Mail showed it twice).
// The send paths now choose the Message-ID header themselves and store it in
// message_id; the provider's id moves to provider_message_id, which is what
// delivery and bounce notifications report back.
//
// message_id and in_reply_to are also normalized to one form, "<id@host>".
// Each source stored its own form (go-message strips the brackets, webhooks
// and headers keep them) and the exact-match lookups missed between the two.
migrate(
    app => {
        const messages = app.findCollectionByNameOrId('mail_messages')
        messages.fields.add(
            new Field({
                id: 'mail_messages_provider_message_id',
                name: 'provider_message_id',
                type: 'text',
                required: false,
            })
        )
        messages.indexes.push(
            'CREATE INDEX `idx_mail_messages_provider_message_id` ON `mail_messages` (`provider_message_id`)'
        )
        app.save(messages)

        // Before this migration a sent message's message_id WAS the provider's
        // id, so copy it across before normalizing adds brackets to it.
        app.db()
            .newQuery(`
                UPDATE mail_messages
                SET provider_message_id = message_id
                WHERE sent_by != '' AND delivery_status != 'draft' AND message_id != ''
            `)
            .execute()

        app.db()
            .newQuery(`
                UPDATE mail_messages
                SET message_id = '<' || TRIM(message_id, ' <>') || '>'
                WHERE TRIM(message_id, ' <>') != ''
            `)
            .execute()
        app.db()
            .newQuery(`
                UPDATE mail_messages
                SET in_reply_to = '<' || TRIM(in_reply_to, ' <>') || '>'
                WHERE TRIM(in_reply_to, ' <>') != ''
            `)
            .execute()
    },
    app => {
        const messages = app.findCollectionByNameOrId('mail_messages')
        messages.indexes = messages.indexes.filter(
            idx => !idx.includes('idx_mail_messages_provider_message_id')
        )
        messages.fields.removeById('mail_messages_provider_message_id')
        app.save(messages)
    }
)
