/// <reference path="../../../server/pb_data/types.d.ts" />
// NOTE: the adminOrOwnerRule here is mirrored in 1713000000_create_mail_collections.js
// (orgAdminRule, applied to fresh installs). Any change to the mail_domains
// write-rule must be reflected in BOTH files — fresh installs run phase-2 of
// 1713000000 exclusively, while upgrades run 1713000011 on top.
migrate(
    app => {
        const memberRule = '@request.auth.id != ""'
        const adminOrOwnerRule =
            '@request.auth.id != "" && (@request.auth.role = "admin" || @request.auth.role = "owner")'

        const domains = app.findCollectionByNameOrId('mail_domains')
        domains.listRule = memberRule
        domains.viewRule = memberRule
        domains.createRule = adminOrOwnerRule
        domains.updateRule = adminOrOwnerRule
        domains.deleteRule = adminOrOwnerRule
        app.save(domains)
    },
    app => {
        const adminOnlyRule = '@request.auth.id != "" && @request.auth.role = "admin"'

        const domains = app.findCollectionByNameOrId('mail_domains')
        domains.createRule = adminOnlyRule
        domains.updateRule = adminOnlyRule
        domains.deleteRule = adminOnlyRule
        app.save(domains)
    }
)
