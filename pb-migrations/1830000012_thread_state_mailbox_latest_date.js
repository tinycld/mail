/// <reference path="../../../server/pb_data/types.d.ts" />

// The thread list is one live query on mail_thread_state. A live query cannot
// sort or filter through a relation, so the two thread columns the list sorts
// and scopes on live on the state row too. Go keeps them equal to the thread's
// (server/thread_state_sync.go): filled on create, latest_date propagated on
// every thread save that changes it.
//
// The backfill coalesces to '' because PocketBase enforces the thread
// relation at the application level, not with a SQLite FOREIGN KEY, so a
// state row left behind by an already-orphaned thread must not abort the
// install; the Go create hook fills both columns for new rows either way.
migrate(
    app => {
        const states = app.findCollectionByNameOrId('mail_thread_state')
        states.fields.add(
            new Field({
                id: 'mail_thr_state_mailbox',
                name: 'mailbox',
                type: 'relation',
                collectionId: 'pbc_mail_mailboxes_01',
                cascadeDelete: true,
                maxSelect: 1,
            })
        )
        states.fields.add(
            new Field({
                id: 'mail_thr_state_latest',
                name: 'latest_date',
                type: 'date',
            })
        )
        states.indexes.push(
            'CREATE INDEX `idx_mail_thread_state_list` ON `mail_thread_state` (`user`, `mailbox`, `folder`, `latest_date`)'
        )
        app.save(states)

        app.db()
            .newQuery(
                `UPDATE mail_thread_state
                 SET mailbox = COALESCE((SELECT t.mailbox FROM mail_threads t WHERE t.id = mail_thread_state.thread), ''),
                     latest_date = COALESCE((SELECT t.latest_date FROM mail_threads t WHERE t.id = mail_thread_state.thread), '')`
            )
            .execute()
    },
    app => {
        const states = app.findCollectionByNameOrId('mail_thread_state')
        states.indexes = states.indexes.filter(idx => !idx.includes('idx_mail_thread_state_list'))
        states.fields.removeById('mail_thr_state_latest')
        states.fields.removeById('mail_thr_state_mailbox')
        app.save(states)
    }
)
