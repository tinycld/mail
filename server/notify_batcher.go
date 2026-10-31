package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/approutes"
	"tinycld.org/core/appstate"
	"tinycld.org/core/logging"
	"tinycld.org/core/notify"
	"tinycld.org/core/readonly"
)

// mailNotificationURL is the in-app deep link for mail notifications.
//
// Built from approutes.Href so the /a app-route prefix has one definition:
// NotificationDrawer pushes a stored url at the router verbatim, so a value
// minted without the prefix lands the reader on +not-found. The migration in
// core (pb_migrations/2010000000_prefix_notification_urls.js) only rewrote
// rows that already existed — every row written from here has to be correct
// on its own.
//
// Single-org: routes carry no [orgSlug] segment, so nothing interpolates into
// this and it stays a fixed path.
var mailNotificationURL = approutes.Href("mail")

var notifyLog = logging.ForPackage("mail")

type pendingMail struct {
	sender  string
	subject string
}

// mailBufferKey holds the app's pending notifications, userID ->
// *mailBucket. Per app: each app's batcher must flush only its own users'
// mail, and a user id is unique only in its app.
var mailBufferKey = appstate.NewKey[*sync.Map]("mail.notifyBuffer")

func mailBufferFor(app core.App) *sync.Map {
	return mailBufferKey.GetOrInit(app, func() *sync.Map { return &sync.Map{} })
}

type mailBucket struct {
	mu    sync.Mutex
	items []pendingMail
}

// bufferMailNotificationForUser adds a mail message to the app's per-user
// buffer for batched delivery. Called from the inbound message hook.
func bufferMailNotificationForUser(app core.App, userID, sender, subject string) {
	val, _ := mailBufferFor(app).LoadOrStore(userID, &mailBucket{})
	buf := val.(*mailBucket)
	buf.mu.Lock()
	buf.items = append(buf.items, pendingMail{
		sender:  sender,
		subject: subject,
	})
	buf.mu.Unlock()
}

// mailBatchInterval is how often the batcher flushes the buffer.
const mailBatchInterval = 2 * time.Minute

const mailNotifyDroppedMsg = "mail notifications dropped: the server stayed read-only past the wait"

// startMailBatcher flushes the buffer every two minutes until ctx ends (the
// app's terminate), and once more then: the buffer lives only in memory, so
// what it still holds would otherwise be lost without a word.
func startMailBatcher(ctx context.Context, app core.App) {
	ticker := time.NewTicker(mailBatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			flushMailBufferWhenWritable(ctx, app)
			return
		case <-ticker.C:
			flushMailBufferWhenWritable(ctx, app)
		}
	}
}

// flushMailBufferWhenWritable flushes the buffer of mail that arrived,
// possibly just before the server went read-only. The notifications are
// writes, so the flush waits for the mode to end, bounded by
// readonly.TailWait and by ctx, which ends when the app terminates. When the
// wait ends first, the buffered notifications are dropped and logged. At the
// terminate ctx has already ended: a writable server flushes at once, a
// read-only one drops.
func flushMailBufferWhenWritable(ctx context.Context, app core.App) {
	waitCtx, cancel := context.WithTimeout(ctx, readonly.TailWait)
	defer cancel()
	err := readonly.WhenWritable(waitCtx, app, func() error {
		flushMailBuffer(ctx, app)
		return nil
	})
	if err != nil {
		users, messages := dropMailBuffer(app)
		if messages > 0 {
			notifyLog.Warn(mailNotifyDroppedMsg, "users", users, "messages", messages, "err", err)
		}
	}
}

// dropMailBuffer empties the app's buffer and reports what it held.
func dropMailBuffer(app core.App) (users, messages int) {
	buffer := mailBufferFor(app)
	buffer.Range(func(key, value any) bool {
		buf := value.(*mailBucket)
		buf.mu.Lock()
		n := len(buf.items)
		buf.items = nil
		buf.mu.Unlock()
		buffer.Delete(key)
		if n > 0 {
			users++
			messages += n
		}
		return true
	})
	return users, messages
}

// flushMailBuffer sends one notification per user for the buffered mail. It
// stops when the server turns read-only in the middle: the users not reached
// yet keep their buffer for the next flush, which waits for the mode to end.
// ctx bounds the push dispatches; the notification rows are written
// regardless.
func flushMailBuffer(ctx context.Context, app core.App) {
	buffer := mailBufferFor(app)
	buffer.Range(func(key, value any) bool {
		if readonly.Active(app) {
			return false
		}
		userID := key.(string)
		buf := value.(*mailBucket)

		buf.mu.Lock()
		items := buf.items
		buf.items = nil
		buf.mu.Unlock()

		buffer.Delete(userID)

		if len(items) == 0 {
			return true
		}

		mode := getMailNotifyMode(app, userID)
		if mode == "important_only" {
			items = filterImportantMail(app, userID, items)
			if len(items) == 0 {
				return true
			}
		}

		params := notify.NotifyParams{
			UserID:  userID,
			Type:    "mail_new_message",
			Package: "mail",
			Title:   fmt.Sprintf("New mail from %s", items[0].sender),
			Body:    items[0].subject,
			URL:     mailNotificationURL,
		}
		if len(items) > 1 {
			params.Title = fmt.Sprintf("You have %d new messages", len(items))
			params.Body = fmt.Sprintf("From %s and others", items[0].sender)
		}
		// DeliverToUserContext logs its own failures.
		_ = notify.DeliverToUserContext(ctx, app, params)

		return true
	})
}

func getMailNotifyMode(app core.App, userID string) string {
	records, err := app.FindRecordsByFilter(
		"user_preferences",
		"user = {:userId} && app = 'notifications' && key = 'mail_notify_mode'",
		"",
		1,
		0,
		map[string]any{"userId": userID},
	)
	if err != nil || len(records) == 0 {
		return "batched"
	}
	// user_preferences.value is a `json` field, so Get returns a
	// types.JSONRaw, not a string — assert on the concrete type and it always
	// misses. Marshal/unmarshal the JSON string instead.
	raw, err := json.Marshal(records[0].Get("value"))
	if err != nil {
		return "batched"
	}
	var mode string
	if json.Unmarshal(raw, &mode) != nil || mode == "" {
		return "batched"
	}
	return mode
}

// filterImportantMail keeps only mail from a known contact, and records a
// silent (no-push) notification for the rest.
//
// The contacts package is optional — a workspace may not have it installed, in
// which case the collection is absent and every sender reads as unknown. That
// degrades to "nothing is important", which is the safe direction: the user
// still gets the silent notifications, just no push.
func filterImportantMail(app core.App, userID string, items []pendingMail) []pendingMail {
	var important []pendingMail
	for _, item := range items {
		contacts, err := app.FindRecordsByFilter(
			"contacts",
			"owner = {:userId} && email ~ {:sender}",
			"",
			1,
			0,
			map[string]any{"userId": userID, "sender": item.sender},
		)
		if err == nil && len(contacts) > 0 {
			important = append(important, item)
			continue
		}

		// Still create a silent notification (no push) for non-important mail
		collection, err := app.FindCollectionByNameOrId("notifications")
		if err != nil {
			continue
		}
		record := core.NewRecord(collection)
		record.Set("user", userID)
		record.Set("type", "mail_new_message")
		record.Set("package", "mail")
		record.Set("title", fmt.Sprintf("New mail from %s", item.sender))
		record.Set("body", item.subject)
		record.Set("url", mailNotificationURL)
		record.Set("read", false)
		record.Set("dismissed", false)
		if err := app.Save(record); err != nil {
			notifyLog.Error("failed to save a silent mail notification", "userID", userID, "error", err)
		}
	}
	return important
}
