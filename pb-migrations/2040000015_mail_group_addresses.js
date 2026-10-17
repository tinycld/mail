/// <reference path="../../../server/pb_data/types.d.ts" />

// Group addresses (design 2026-10-17, phase 4). An address can deliver to a
// group instead of an inbox: mail to it reaches every member's own inbox.
//
// A row holds exactly one target, mailbox or group. PocketBase checks a
// required field before any hook runs, so mailbox is now optional and the
// server's create hook (server/group_addresses.go) enforces the rule.
// cascadeDelete is true on group: an address of a deleted group has nowhere
// to deliver. The mail it delivered stays; PocketBase clears delivered_to.
migrate(
    app => {
        const addresses = app.findCollectionByNameOrId('mail_addresses')
        addresses.fields.getById('mail_addr_mailbox').required = false
        addresses.fields.add(
            new Field({
                id: 'mail_addr_group',
                name: 'group',
                type: 'relation',
                collectionId: 'pbc_groups_01',
                cascadeDelete: true,
                maxSelect: 1,
                required: false,
            })
        )
        addresses.indexes.push('CREATE INDEX `idx_mail_addresses_group` ON `mail_addresses` (`group`)')
        app.save(addresses)
    },
    app => {
        // A group row has no inbox, so mailbox cannot be required while one
        // exists. Raw SQL skips the relation cleanup, so clear the messages'
        // references first, as a record delete would.
        const groupRows = "SELECT id FROM mail_addresses WHERE `group` != ''"
        app.db().newQuery(`UPDATE mail_messages SET delivered_to = '' WHERE delivered_to IN (${groupRows})`).execute()
        app.db().newQuery(`UPDATE mail_messages SET sent_from = '' WHERE sent_from IN (${groupRows})`).execute()
        app.db().newQuery("DELETE FROM mail_addresses WHERE `group` != ''").execute()
        const addresses = app.findCollectionByNameOrId('mail_addresses')
        addresses.indexes = addresses.indexes.filter(index => !index.includes('idx_mail_addresses_group'))
        addresses.fields.removeById('mail_addr_group')
        addresses.fields.getById('mail_addr_mailbox').required = true
        app.save(addresses)
    }
)
