/// <reference path="../pb_data/types.d.ts" />
migrate(
    (app) => {
        const collection = new Collection({
            id: 'pbc_orgs_00001',
            name: 'orgs',
            type: 'base',
            system: false,
            listRule: '',
            viewRule: '',
            fields: [
                {
                    id: 'orgs_name',
                    name: 'name',
                    type: 'text',
                    required: true,
                    min: 3,
                    max: 45,
                },
                {
                    id: 'orgs_slug',
                    name: 'slug',
                    type: 'text',
                    required: true,
                    min: 3,
                    max: 15,
                    pattern: '^[a-z0-9]+(?:-[a-z0-9]+)*$',
                },
                {
                    id: 'orgs_logo',
                    name: 'logo',
                    type: 'file',
                    required: false,
                    maxSelect: 1,
                    maxSize: 5242880,
                    mimeTypes: [
                        'image/jpeg',
                        'image/png',
                        'image/svg+xml',
                        'image/gif',
                        'image/webp',
                    ],
                },
                {
                    id: 'orgs_users',
                    name: 'users',
                    type: 'relation',
                    required: false,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: false,
                    maxSelect: 999,
                },
                {
                    id: 'orgs_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'orgs_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_orgs_slug` ON `orgs` (`slug`)',
            ],
        })

        return app.save(collection)
    },
    (app) => {
        const collection = app.findCollectionByNameOrId('orgs')
        return app.delete(collection)
    },
)
