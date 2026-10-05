/// <reference path="../../../server/pb_data/types.d.ts" />

// mail_folder_counts was a VIEW. PocketBase emits no realtime events for a
// view, so the sidebar refetched it whenever a local thread-state row changed,
// and a message that arrived while the user was looking at another folder left
// the badge stale. It is now a base collection with one row per (user,
// mailbox), recomputed by a Go hook on every mail_thread_state change
// (server/folder_counts.go), so realtime covers it like any other table. It
// also carries per-folder totals for the list's page indicator.
//
// id is `user || mailbox` (two 15-char ids), so a recompute addresses its row
// without a lookup.
migrate(
    app => {
        app.delete(app.findCollectionByNameOrId('mail_folder_counts'))

        const authed = '@request.auth.id != ""'
        const number = (id, name) => ({ id, name, type: 'number', onlyInt: true })
        const counts = new Collection({
            id: 'pbc_mail_folder_counts_02',
            name: 'mail_folder_counts',
            type: 'base',
            system: false,
            listRule: `${authed} && user = @request.auth.id`,
            viewRule: `${authed} && user = @request.auth.id`,
            createRule: null,
            updateRule: null,
            deleteRule: null,
            fields: [
                {
                    id: 'mail_fc_id',
                    name: 'id',
                    type: 'text',
                    primaryKey: true,
                    required: true,
                    system: true,
                    min: 30,
                    max: 30,
                    pattern: '^[a-z0-9]{30}$',
                    autogeneratePattern: '',
                },
                {
                    id: 'mail_fc_user',
                    name: 'user',
                    type: 'relation',
                    required: true,
                    collectionId: '_pb_users_auth_',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                {
                    id: 'mail_fc_mailbox',
                    name: 'mailbox',
                    type: 'relation',
                    required: true,
                    collectionId: 'pbc_mail_mailboxes_01',
                    cascadeDelete: true,
                    maxSelect: 1,
                },
                number('mail_fc_inbox', 'inbox'),
                number('mail_fc_inbox_total', 'inbox_total'),
                number('mail_fc_archive', 'archive'),
                number('mail_fc_drafts', 'drafts'),
                number('mail_fc_sent', 'sent'),
                number('mail_fc_starred', 'starred'),
                number('mail_fc_trash', 'trash'),
                number('mail_fc_spam', 'spam'),
                number('mail_fc_total', 'total'),
                { id: 'mail_fc_created', name: 'created', type: 'autodate', onCreate: true, onUpdate: false },
                { id: 'mail_fc_updated', name: 'updated', type: 'autodate', onCreate: true, onUpdate: true },
            ],
            indexes: ['CREATE UNIQUE INDEX `idx_mail_folder_counts_user_mailbox` ON `mail_folder_counts` (`user`, `mailbox`)'],
        })
        app.save(counts)

        // 1830000012 filled mail_thread_state.mailbox, so no join is needed.
        app.db()
            .newQuery(
                `INSERT INTO mail_folder_counts
                    (id, user, mailbox, inbox, inbox_total, archive, drafts, sent, starred, trash, spam, total, created, updated)
                 SELECT
                    (s.user || s.mailbox),
                    s.user,
                    s.mailbox,
                    SUM(CASE WHEN s.folder = 'inbox' AND s.is_read = 0 THEN 1 ELSE 0 END),
                    SUM(CASE WHEN s.folder = 'inbox' THEN 1 ELSE 0 END),
                    SUM(CASE WHEN s.folder = 'archive' THEN 1 ELSE 0 END),
                    SUM(CASE WHEN s.folder = 'drafts' THEN 1 ELSE 0 END),
                    SUM(CASE WHEN s.is_sent AND s.folder NOT IN ('trash', 'spam') THEN 1 ELSE 0 END),
                    SUM(CASE WHEN s.is_starred THEN 1 ELSE 0 END),
                    SUM(CASE WHEN s.folder = 'trash' THEN 1 ELSE 0 END),
                    SUM(CASE WHEN s.folder = 'spam' THEN 1 ELSE 0 END),
                    COUNT(*),
                    strftime('%Y-%m-%d %H:%M:%fZ', 'now'),
                    strftime('%Y-%m-%d %H:%M:%fZ', 'now')
                 FROM mail_thread_state s
                 WHERE s.mailbox != ''
                 GROUP BY s.user, s.mailbox`
            )
            .execute()
    },
    app => {
        app.delete(app.findCollectionByNameOrId('mail_folder_counts'))
        const view = new Collection({
            id: 'pbc_mail_folder_counts_01',
            name: 'mail_folder_counts',
            type: 'view',
            system: false,
            listRule: '@request.auth.id != "" && user ?= @request.auth.id',
            viewRule: '@request.auth.id != "" && user ?= @request.auth.id',
            viewQuery: `
                SELECT
                    (s.user || ':' || t.mailbox) AS id,
                    s.user AS user,
                    t.mailbox AS mailbox,
                    SUM(CASE WHEN s.folder = 'inbox' AND s.is_read = 0 THEN 1 ELSE 0 END) AS inbox,
                    SUM(CASE WHEN s.folder = 'drafts' THEN 1 ELSE 0 END) AS drafts,
                    SUM(CASE WHEN s.is_sent AND s.folder NOT IN ('trash', 'spam') THEN 1 ELSE 0 END) AS sent,
                    SUM(CASE WHEN s.is_starred THEN 1 ELSE 0 END) AS starred,
                    SUM(CASE WHEN s.folder = 'trash' THEN 1 ELSE 0 END) AS trash,
                    SUM(CASE WHEN s.folder = 'spam' THEN 1 ELSE 0 END) AS spam
                FROM mail_thread_state s
                JOIN mail_threads t ON s.thread = t.id
                GROUP BY s.user, t.mailbox
            `,
        })
        app.save(view)
    }
)
