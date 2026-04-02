/// <reference path="../pb_data/types.d.ts" />
migrate(
    (app) => {
        const orgs = app.findCollectionByNameOrId('orgs')
        orgs.listRule = 'user_org_via_org.user ?= @request.auth.id'
        orgs.viewRule = 'user_org_via_org.user ?= @request.auth.id'
        return app.save(orgs)
    },
    (app) => {
        const orgs = app.findCollectionByNameOrId('orgs')
        orgs.listRule = ''
        orgs.viewRule = ''
        return app.save(orgs)
    },
)
