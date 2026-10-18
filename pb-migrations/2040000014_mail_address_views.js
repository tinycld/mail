/// <reference path="../pb_data/types.d.ts" />
// Address views (design 2026-10-15, phase 3C). A person with several
// addresses on one inbox can read the inbox as one list per address.
//
//   - mail_threads.address_keys and mail_thread_state.address_keys hold the
//     ids of every address a thread used (delivered_to or sent_from of any
//     message), space separated. A view filters rows with `address_keys ~ id`.
//     No index: idx_mail_thread_state_list (user, mailbox, folder,
//     latest_date) already narrows the rows before the text test.
//   - mail_identities.address_views turns the per-address views on for a
//     person; hidden_address_views lists the addresses that person hides.
//   - mail_folder_counts.inbox_by_address holds unread inbox counts per address.
//
// The backfill derives address_keys from the existing messages in two
// statements. The inner UNION removes duplicate ids.

migrate(
    app => {
        const threads = app.findCollectionByNameOrId('mail_threads')
        threads.fields.add(new Field({ id: 'mail_threads_address_keys', name: 'address_keys', type: 'text', max: 2000 }))
        app.save(threads)

        const states = app.findCollectionByNameOrId('mail_thread_state')
        states.fields.add(
            new Field({ id: 'mail_thr_state_address_keys', name: 'address_keys', type: 'text', max: 2000 })
        )
        app.save(states)

        const identities = app.findCollectionByNameOrId('mail_identities')
        identities.fields.add(new Field({ id: 'mail_identities_address_views', name: 'address_views', type: 'bool' }))
        identities.fields.add(
            new Field({
                id: 'mail_identities_hidden_address_views',
                name: 'hidden_address_views',
                type: 'relation',
                collectionId: 'pbc_mail_addresses_01',
                cascadeDelete: false,
                maxSelect: 999,
                required: false,
            })
        )
        app.save(identities)

        const counts = app.findCollectionByNameOrId('mail_folder_counts')
        counts.fields.add(
            new Field({ id: 'mail_folder_counts_inbox_by_address', name: 'inbox_by_address', type: 'json', maxSize: 200000 })
        )
        app.save(counts)

        app.db()
            .newQuery(
                `UPDATE mail_threads SET address_keys = COALESCE((
                    SELECT group_concat(id, ' ') FROM (
                        SELECT delivered_to AS id FROM mail_messages WHERE thread = mail_threads.id AND delivered_to != ''
                        UNION
                        SELECT sent_from AS id FROM mail_messages WHERE thread = mail_threads.id AND sent_from != ''
                    )
                ), '')`
            )
            .execute()
        app.db()
            .newQuery(
                `UPDATE mail_thread_state SET address_keys = COALESCE(
                    (SELECT address_keys FROM mail_threads WHERE mail_threads.id = mail_thread_state.thread), '')`
            )
            .execute()
    },
    app => {
        const counts = app.findCollectionByNameOrId('mail_folder_counts')
        counts.fields.removeById('mail_folder_counts_inbox_by_address')
        app.save(counts)

        const identities = app.findCollectionByNameOrId('mail_identities')
        identities.fields.removeById('mail_identities_hidden_address_views')
        identities.fields.removeById('mail_identities_address_views')
        app.save(identities)

        const states = app.findCollectionByNameOrId('mail_thread_state')
        states.fields.removeById('mail_thr_state_address_keys')
        app.save(states)

        const threads = app.findCollectionByNameOrId('mail_threads')
        threads.fields.removeById('mail_threads_address_keys')
        app.save(threads)
    }
)
