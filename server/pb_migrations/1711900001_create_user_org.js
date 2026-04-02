/// <reference path="../pb_data/types.d.ts" />
migrate(
    (app) => {
        const collection = new Collection({
            id: 'pbc_user_org_01',
            name: 'user_org',
            type: 'base',
            system: false,
            listRule: 'user = @request.auth.id',
            viewRule: 'user = @request.auth.id',
            fields: [
                {
                    id: 'user_org_org',
                    name: 'org',
                    type: 'relation',
                    required: true,
                    collectionId: 'pbc_orgs_00001',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'user_org_user',
                    name: 'user',
                    type: 'relation',
                    required: true,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'user_org_role',
                    name: 'role',
                    type: 'select',
                    required: true,
                    values: ['admin', 'member'],
                    maxSelect: 1,
                },
                {
                    id: 'user_org_created',
                    name: 'created',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: false,
                },
                {
                    id: 'user_org_updated',
                    name: 'updated',
                    type: 'autodate',
                    onCreate: true,
                    onUpdate: true,
                },
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_user_org_unique` ON `user_org` (`org`, `user`)',
            ],
        })

        return app.save(collection)
    },
    (app) => {
        const collection = app.findCollectionByNameOrId('user_org')
        return app.delete(collection)
    },
)
