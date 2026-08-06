/// <reference path="../../../server/pb_data/types.d.ts" />

// mail_messages never had a recipients_bcc column, so the draft endpoint's
// write to it was silently dropped and a reopened draft lost its BCC list
// (and a sent message's own stored copy never recorded who was BCC'd —
// delivered headers are unaffected either way). Add the column to match
// recipients_to / recipients_cc; the send + draft write paths now persist it.
migrate(
    app => {
        const messages = app.findCollectionByNameOrId('mail_messages')

        messages.fields.add(
            new Field({
                id: 'mail_messages_recipients_bcc',
                name: 'recipients_bcc',
                type: 'json',
                required: false,
            })
        )

        app.save(messages)
    },
    app => {
        const messages = app.findCollectionByNameOrId('mail_messages')
        messages.fields.removeById('mail_messages_recipients_bcc')
        app.save(messages)
    }
)
