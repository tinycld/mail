/// <reference path="../../../server/pb_data/types.d.ts" />

// sent_by attributes an outgoing message to the human who sent it.
//
// A mailbox is shared, so sender_email (derived from the mailbox or alias)
// cannot identify the person — every member of shared@example.com sends as
// the same address. Without this field the question "who sent these 5,000
// messages?" has no answer at all, which is what a provider abuse complaint,
// a per-user send cap, and an offboarding review each need first.
//
// Empty on inbound messages (nobody here sent them) and on messages stored
// before this migration, so a reader must treat "" as unknown rather than
// as a claim about who sent it.
//
// cascadeDelete is false on purpose: deleting a user must not delete the
// mail they sent. The reference is left dangling instead — attribution that
// disappears when the actor leaves is worthless for the cases above.
migrate(
    app => {
        const messages = app.findCollectionByNameOrId('mail_messages')
        const users = app.findCollectionByNameOrId('users')

        messages.fields.add(
            new Field({
                id: 'mail_messages_sent_by',
                name: 'sent_by',
                type: 'relation',
                required: false,
                collectionId: users.id,
                cascadeDelete: false,
                maxSelect: 1,
            })
        )

        app.save(messages)
    },
    app => {
        const messages = app.findCollectionByNameOrId('mail_messages')
        messages.fields.removeById('mail_messages_sent_by')
        app.save(messages)
    }
)
