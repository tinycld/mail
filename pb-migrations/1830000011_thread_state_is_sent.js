/// <reference path="../../../server/pb_data/types.d.ts" />

// Sent becomes a flag on the thread, not a folder the thread is moved to.
//
// A thread has one folder per user. Sending a reply set that folder to
// 'sent', so replying to a received message moved the whole conversation out
// of the Inbox — it then showed only in All Mail and Sent. is_sent records
// "this user sent a message in this thread" alongside the folder, the way
// is_starred does, and the Sent view reads it. Replying now leaves the folder
// alone; a thread the user starts is still filed under 'sent' so it stays
// out of the Inbox until someone replies.
//
// mail_folder_counts is rebuilt so its sent count matches the Sent view:
// flagged threads, except ones in trash or spam.
migrate(
    app => {
        const states = app.findCollectionByNameOrId('mail_thread_state')
        states.fields.add(
            new Field({
                id: 'mail_thr_state_is_sent',
                name: 'is_sent',
                type: 'bool',
            })
        )
        app.save(states)

        app.db().newQuery(`UPDATE mail_thread_state SET is_sent = 1 WHERE folder = 'sent'`).execute()

        const counts = app.findCollectionByNameOrId('mail_folder_counts')
        counts.viewQuery = `
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
        `
        app.save(counts)
    },
    app => {
        const counts = app.findCollectionByNameOrId('mail_folder_counts')
        counts.viewQuery = `
            SELECT
                (s.user || ':' || t.mailbox) AS id,
                s.user AS user,
                t.mailbox AS mailbox,
                SUM(CASE WHEN s.folder = 'inbox' AND s.is_read = 0 THEN 1 ELSE 0 END) AS inbox,
                SUM(CASE WHEN s.folder = 'drafts' THEN 1 ELSE 0 END) AS drafts,
                SUM(CASE WHEN s.folder = 'sent' THEN 1 ELSE 0 END) AS sent,
                SUM(CASE WHEN s.is_starred THEN 1 ELSE 0 END) AS starred,
                SUM(CASE WHEN s.folder = 'trash' THEN 1 ELSE 0 END) AS trash,
                SUM(CASE WHEN s.folder = 'spam' THEN 1 ELSE 0 END) AS spam
            FROM mail_thread_state s
            JOIN mail_threads t ON s.thread = t.id
            GROUP BY s.user, t.mailbox
        `
        app.save(counts)

        const states = app.findCollectionByNameOrId('mail_thread_state')
        states.fields.removeById('mail_thr_state_is_sent')
        app.save(states)
    }
)
