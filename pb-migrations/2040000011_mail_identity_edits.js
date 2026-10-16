/// <reference path="../pb_data/types.d.ts" />
// A person edits their own Default From; an admin edits any person's email
// name and Default From. The server hooks (server/identity_edits.go) keep
// user, inbox and retired fixed and check what may change.
const authed = '@request.auth.id != ""'
const admin = '(@request.auth.role = "admin" || @request.auth.role = "owner")'

migrate(
    app => {
        const identities = app.findCollectionByNameOrId('mail_identities')
        identities.updateRule = `${authed} && @request.auth.disabled != true && (user = @request.auth.id || ${admin})`
        app.save(identities)
    },
    app => {
        const identities = app.findCollectionByNameOrId('mail_identities')
        identities.updateRule = null
        app.save(identities)
    }
)
