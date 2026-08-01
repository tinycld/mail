/// <reference path="../../../server/pb_data/types.d.ts" />
migrate(
    app => {
        const mailboxes = app.findCollectionByNameOrId('mail_mailboxes')

        mailboxes.fields.add(
            new Field({
                id: 'mail_mailboxes_name',
                name: 'name',
                type: 'text',
                required: false,
                max: 100,
            })
        )

        app.save(mailboxes)
    },
    app => {
        const mailboxes = app.findCollectionByNameOrId('mail_mailboxes')
        mailboxes.fields.removeById('mail_mailboxes_name')
        app.save(mailboxes)
    }
)
