/// <reference path="../../../server/pb_data/types.d.ts" />

// is_auto_submitted marks a message that was sent automatically: an
// auto-reply, mailing-list mail, or mail that a rule sent. The server sets it
// from the message's headers when it stores the message (isAutoSubmitted in
// server/mail_loops.go), and rules do not forward or answer such a message,
// so two automatic senders cannot answer each other in a loop.
//
// No backfill: inbound webhook and SMTP messages never stored their headers,
// so older rows have nothing to read the flag from.
migrate(
    app => {
        const messages = app.findCollectionByNameOrId('mail_messages')
        messages.fields.add(
            new Field({
                id: 'mail_messages_is_auto_submitted',
                name: 'is_auto_submitted',
                type: 'bool',
            })
        )
        app.save(messages)
    },
    app => {
        const messages = app.findCollectionByNameOrId('mail_messages')
        messages.fields.removeById('mail_messages_is_auto_submitted')
        app.save(messages)
    }
)
