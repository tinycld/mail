/// <reference path="../../../server/pb_data/types.d.ts" />

// Domains for members of groups (design 2026-10-15, phase 2). Named after
// core's 2040000000_create_groups so pbc_groups_01 exists when this runs.
//
// coverage 'groups' gives <email name>@domain only to members of
// coverage_groups. via_group records which group gave a rule address, so the
// UI can say why a person has it and what they lose when they leave.
// cascadeDelete is false on both relations: deleting a group must not delete
// a domain or an address row; PocketBase clears the reference and the
// reconciler removes the addresses the group gave.
migrate(
    app => {
        const domains = app.findCollectionByNameOrId('mail_domains')
        const coverage = domains.fields.getById('mail_domains_coverage')
        coverage.values = ['everyone', 'groups', 'manual']
        domains.fields.add(
            new Field({
                id: 'mail_domains_coverage_groups',
                name: 'coverage_groups',
                type: 'relation',
                required: false,
                collectionId: 'pbc_groups_01',
                cascadeDelete: false,
                maxSelect: 999,
            })
        )
        app.save(domains)

        const addresses = app.findCollectionByNameOrId('mail_addresses')
        addresses.fields.add(
            new Field({
                id: 'mail_addr_via_group',
                name: 'via_group',
                type: 'relation',
                required: false,
                collectionId: 'pbc_groups_01',
                cascadeDelete: false,
                maxSelect: 1,
            })
        )
        app.save(addresses)
    },
    app => {
        app.db().newQuery(`UPDATE mail_domains SET coverage = 'manual' WHERE coverage = 'groups'`).execute()
        const addresses = app.findCollectionByNameOrId('mail_addresses')
        addresses.fields.removeById('mail_addr_via_group')
        app.save(addresses)
        const domains = app.findCollectionByNameOrId('mail_domains')
        domains.fields.removeById('mail_domains_coverage_groups')
        domains.fields.getById('mail_domains_coverage').values = ['everyone', 'manual']
        app.save(domains)
    }
)
