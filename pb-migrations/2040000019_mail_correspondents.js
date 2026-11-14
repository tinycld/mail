/// <reference path="../pb_data/types.d.ts" />
// Addresses each user has sent mail to, so "has sent email to before" is one
// indexed read instead of a scan of every sent message's recipient JSON.
// Server-written only.
migrate(
    app => {
        const users = app.findCollectionByNameOrId('users')
        app.save(
            new Collection({
                id: 'pbc_mail_correspondents',
                name: 'mail_correspondents',
                type: 'base',
                system: false,
                listRule: null,
                viewRule: null,
                createRule: null,
                updateRule: null,
                deleteRule: null,
                fields: [
                    { id: 'mcor_user', name: 'user', type: 'relation', required: true, collectionId: users.id, cascadeDelete: true, maxSelect: 1 },
                    { id: 'mcor_email', name: 'email', type: 'text', required: true, max: 320 },
                    { id: 'mcor_last_sent', name: 'last_sent', type: 'date' },
                ],
                indexes: ['CREATE UNIQUE INDEX `idx_mail_correspondents_user_email` ON `mail_correspondents` (`user`, `email`)'],
            })
        )
    },
    app => {
        app.delete(app.findCollectionByNameOrId('mail_correspondents'))
    }
)
