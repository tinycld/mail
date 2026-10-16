/// <reference path="../../../server/pb_data/types.d.ts" />

// Addresses become their own rows (design: 2026-10-15 mail addresses and
// domain rules, phase 1).
//
// Before this, an address was the mailbox's own address/domain columns plus
// mail_mailbox_aliases, and an alias could only use its mailbox's domain. So a
// second domain gave nobody an address on it. mail_addresses holds every
// (local_part, domain) → mailbox mapping, on any domain, and is the one table
// the server reads to route, send and log in.
//
// mail_identities ties a person to their personal inbox and their email name:
// the local part every `everyone` domain gives them. The reconciler
// (server/rule_addresses.go) writes those rows with source = 'rule'.
//
// Existing data is copied, not converted: each mailbox's own address becomes
// its primary row and each alias a manual row, so every address that worked
// before still works. Only the oldest verified domain gets coverage =
// 'everyone', which is exactly today's behavior (new users got an address on
// that domain only). Identities are created by the server at boot, not here,
// because choosing an email name needs the Go collision rules.
migrate(
    app => {
        const authed = '@request.auth.id != ""'
        const admin = `${authed} && (@request.auth.role = "admin" || @request.auth.role = "owner")`
        const autodate = prefix => [
            { id: `${prefix}_created`, name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
            { id: `${prefix}_updated`, name: 'updated', type: 'autodate', onCreate: true, onUpdate: true },
        ]

        const domains = app.findCollectionByNameOrId('mail_domains')
        domains.fields.add(
            new Field({
                id: 'mail_domains_coverage',
                name: 'coverage',
                type: 'select',
                required: false,
                maxSelect: 1,
                values: ['everyone', 'manual'],
            })
        )
        domains.fields.add(
            new Field({ id: 'mail_domains_rank', name: 'rank', type: 'number', required: false, onlyInt: true, min: 0 })
        )
        app.save(domains)

        const addresses = new Collection({
            id: 'pbc_mail_addresses_01',
            name: 'mail_addresses',
            type: 'base',
            system: false,
            listRule: `${authed} && @request.auth.role != "guest"`,
            viewRule: `${authed} && @request.auth.role != "guest"`,
            createRule: admin,
            updateRule: null,
            deleteRule: admin,
            fields: [
                {
                    id: 'mail_addr_local_part',
                    name: 'local_part',
                    type: 'text',
                    required: true,
                    min: 1,
                    max: 64,
                    pattern: '^[a-z0-9._-]+$',
                },
                {
                    id: 'mail_addr_domain',
                    name: 'domain',
                    type: 'relation',
                    required: true,
                    collectionId: 'pbc_mail_domains_01',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'mail_addr_mailbox',
                    name: 'mailbox',
                    type: 'relation',
                    required: true,
                    collectionId: 'pbc_mail_mailboxes_01',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'mail_addr_source',
                    name: 'source',
                    type: 'select',
                    required: true,
                    maxSelect: 1,
                    values: ['manual', 'rule'],
                },
                { id: 'mail_addr_is_primary', name: 'is_primary', type: 'bool' },
                ...autodate('mail_addr'),
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_mail_addresses_local_domain` ON `mail_addresses` (`local_part`, `domain`)',
                'CREATE INDEX `idx_mail_addresses_mailbox` ON `mail_addresses` (`mailbox`)',
            ],
        })
        app.save(addresses)

        const identities = new Collection({
            id: 'pbc_mail_identities_01',
            name: 'mail_identities',
            type: 'base',
            system: false,
            listRule: `${authed} && (user = @request.auth.id || @request.auth.role = "admin" || @request.auth.role = "owner")`,
            viewRule: `${authed} && (user = @request.auth.id || @request.auth.role = "admin" || @request.auth.role = "owner")`,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'mail_ident_user',
                    name: 'user',
                    type: 'relation',
                    required: true,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                // Not required and no cascade: offboarding deletes the leaver's
                // inbox but keeps the identity, marked retired, so nothing
                // gives the anonymized account a new inbox.
                {
                    id: 'mail_ident_inbox',
                    name: 'inbox',
                    type: 'relation',
                    required: false,
                    collectionId: 'pbc_mail_mailboxes_01',
                    cascadeDelete: false,
                    maxSelect: 1,
                },
                {
                    id: 'mail_ident_email_name',
                    name: 'email_name',
                    type: 'text',
                    required: true,
                    min: 1,
                    max: 64,
                    pattern: '^[a-z0-9._-]+$',
                },
                {
                    id: 'mail_ident_default_from',
                    name: 'default_from',
                    type: 'relation',
                    required: false,
                    collectionId: 'pbc_mail_addresses_01',
                    cascadeDelete: false,
                    maxSelect: 1,
                },
                { id: 'mail_ident_retired', name: 'retired', type: 'bool' },
                ...autodate('mail_ident'),
            ],
            indexes: [
                'CREATE UNIQUE INDEX `idx_mail_identities_user` ON `mail_identities` (`user`)',
                "CREATE UNIQUE INDEX `idx_mail_identities_inbox` ON `mail_identities` (`inbox`) WHERE `inbox` != ''",
                'CREATE UNIQUE INDEX `idx_mail_identities_email_name` ON `mail_identities` (`email_name`)',
            ],
        })
        app.save(identities)

        // cascadeDelete false: removing an address must not delete the mail
        // that was delivered to or sent from it.
        const messages = app.findCollectionByNameOrId('mail_messages')
        for (const name of ['delivered_to', 'sent_from']) {
            messages.fields.add(
                new Field({
                    id: `mail_messages_${name}`,
                    name,
                    type: 'relation',
                    required: false,
                    collectionId: 'pbc_mail_addresses_01',
                    cascadeDelete: false,
                    maxSelect: 1,
                })
            )
        }
        app.save(messages)

        // Legacy table: the web client writes mail_addresses now. Create and
        // delete stay open for released clients; the server mirrors them.
        const aliases = app.findCollectionByNameOrId('mail_mailbox_aliases')
        aliases.updateRule = null
        app.save(aliases)

        app.db().newQuery(`UPDATE mail_domains SET coverage = 'manual'`).execute()
        app.db()
            .newQuery(
                `UPDATE mail_domains SET coverage = 'everyone' WHERE id = (
                    SELECT id FROM mail_domains WHERE verified = TRUE ORDER BY created, id LIMIT 1)`
            )
            .execute()
        // The everyone domain is rank 1, so it stays the main domain new
        // inboxes are created on; the others follow by age.
        app.db()
            .newQuery(
                `UPDATE mail_domains SET \`rank\` = (
                    SELECT COUNT(*) FROM mail_domains d2
                    WHERE (d2.coverage = 'everyone') > (mail_domains.coverage = 'everyone')
                       OR ((d2.coverage = 'everyone') = (mail_domains.coverage = 'everyone') AND (
                            d2.created < mail_domains.created
                            OR (d2.created = mail_domains.created AND d2.id <= mail_domains.id))))`
            )
            .execute()

        // Legacy addresses are copied as they are, even if they hold characters
        // outside ^[a-z0-9._-]+$ (mail_mailboxes.address has no pattern): mail
        // must keep arriving. A server Save of such a row would fail the
        // local_part validation, so only raw SQL can write it.
        //
        // PocketBase ids are 15 lowercase alphanumerics; 8 random bytes in hex
        // give 16, cut to 15. INSERT OR IGNORE because (address, domain) is
        // unique on mailboxes and the old hooks kept aliases off those
        // addresses, so a collision here is a pre-existing data error that
        // must not abort the upgrade. The oldest row of a collision keeps the
        // address (ORDER BY created).
        const now = `strftime('%Y-%m-%d %H:%M:%fZ', 'now')`
        const newId = `substr(lower(hex(randomblob(8))), 1, 15)`
        const warnDropped = (what, sql) => {
            const row = new DynamicModel({ n: 0 })
            app.db().newQuery(sql).one(row)
            if (row.n > 0) {
                app.logger().warn(`mail_addresses backfill: dropped ${what} that collide with another address`, 'count', row.n)
            }
        }
        warnDropped(
            'mailboxes',
            `SELECT COUNT(*) AS n FROM mail_mailboxes WHERE trim(address) != '' AND domain != '' AND id NOT IN (
                SELECT MIN(id) FROM mail_mailboxes WHERE trim(address) != '' AND domain != ''
                GROUP BY lower(trim(address)), domain)`
        )
        app.db()
            .newQuery(
                `INSERT OR IGNORE INTO mail_addresses (id, local_part, domain, mailbox, source, is_primary, created, updated)
                 SELECT ${newId}, lower(trim(address)), domain, id, 'manual', TRUE, ${now}, ${now}
                 FROM mail_mailboxes WHERE trim(address) != '' AND domain != ''
                 ORDER BY created, id`
            )
            .execute()
        warnDropped(
            'aliases',
            `SELECT COUNT(*) AS n FROM mail_mailbox_aliases a JOIN mail_mailboxes mb ON mb.id = a.mailbox
             WHERE trim(a.address) != '' AND mb.domain != '' AND (
                EXISTS (SELECT 1 FROM mail_addresses x WHERE x.local_part = lower(trim(a.address)) AND x.domain = mb.domain)
                OR a.id NOT IN (
                    SELECT MIN(a2.id) FROM mail_mailbox_aliases a2 JOIN mail_mailboxes m2 ON m2.id = a2.mailbox
                    WHERE trim(a2.address) != '' AND m2.domain != '' GROUP BY lower(trim(a2.address)), m2.domain))`
        )
        app.db()
            .newQuery(
                `INSERT OR IGNORE INTO mail_addresses (id, local_part, domain, mailbox, source, is_primary, created, updated)
                 SELECT ${newId}, lower(trim(a.address)), mb.domain, a.mailbox, 'manual', FALSE, ${now}, ${now}
                 FROM mail_mailbox_aliases a JOIN mail_mailboxes mb ON mb.id = a.mailbox
                 WHERE trim(a.address) != '' AND mb.domain != ''
                 ORDER BY a.created, a.id`
            )
            .execute()

        // A message sent or drafted from an alias records the alias, not the
        // address. Without sent_from a reopened draft would fall back to the
        // default sender, so the copied manual row becomes its sent_from. An
        // alias that collided above has no row on its own mailbox and stays
        // unset.
        app.db()
            .newQuery(
                `UPDATE mail_messages SET sent_from = (
                    SELECT x.id FROM mail_mailbox_aliases a
                    JOIN mail_mailboxes mb ON mb.id = a.mailbox
                    JOIN mail_addresses x ON x.local_part = lower(trim(a.address))
                        AND x.domain = mb.domain AND x.mailbox = a.mailbox
                    WHERE a.id = mail_messages.alias)
                 WHERE alias != '' AND (sent_from = '' OR sent_from IS NULL) AND EXISTS (
                    SELECT 1 FROM mail_mailbox_aliases a
                    JOIN mail_mailboxes mb ON mb.id = a.mailbox
                    JOIN mail_addresses x ON x.local_part = lower(trim(a.address))
                        AND x.domain = mb.domain AND x.mailbox = a.mailbox
                    WHERE a.id = mail_messages.alias)`
            )
            .execute()
    },
    app => {
        const aliases = app.findCollectionByNameOrId('mail_mailbox_aliases')
        aliases.updateRule = '@request.auth.id != "" && (@request.auth.role = "admin" || @request.auth.role = "owner")'
        app.save(aliases)

        const messages = app.findCollectionByNameOrId('mail_messages')
        messages.fields.removeById('mail_messages_delivered_to')
        messages.fields.removeById('mail_messages_sent_from')
        app.save(messages)

        app.delete(app.findCollectionByNameOrId('mail_identities'))
        app.delete(app.findCollectionByNameOrId('mail_addresses'))

        const domains = app.findCollectionByNameOrId('mail_domains')
        domains.fields.removeById('mail_domains_coverage')
        domains.fields.removeById('mail_domains_rank')
        app.save(domains)
    }
)
