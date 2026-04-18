/// <reference path="../../../server/pb_data/types.d.ts" />
migrate(
    app => {
        const memberRule = 'org.user_org_via_org.user ?= @request.auth.id'
        const adminOrOwnerRule =
            'org.user_org_via_org.user ?= @request.auth.id && (org.user_org_via_org.role ?= "admin" || org.user_org_via_org.role ?= "owner")'

        const domains = app.findCollectionByNameOrId('mail_domains')
        domains.listRule = memberRule
        domains.viewRule = memberRule
        domains.createRule = adminOrOwnerRule
        domains.updateRule = adminOrOwnerRule
        domains.deleteRule = adminOrOwnerRule
        app.save(domains)
    },
    app => {
        const adminOnlyRule =
            'org.user_org_via_org.user ?= @request.auth.id && org.user_org_via_org.role ?= "admin"'

        const domains = app.findCollectionByNameOrId('mail_domains')
        domains.createRule = adminOnlyRule
        domains.updateRule = adminOnlyRule
        domains.deleteRule = adminOnlyRule
        app.save(domains)
    }
)
