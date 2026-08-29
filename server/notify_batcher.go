package mail

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/approutes"
	"tinycld.org/core/notify"
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

type pendingMail struct {
	sender  string
	subject string
}

var mailBuffer sync.Map // map[string]*mailBucket

type mailBucket struct {
	mu    sync.Mutex
	items []pendingMail
}

// bufferMailNotificationForUser adds a mail message to the per-user buffer
// for batched delivery. Called from the inbound message hook.
func bufferMailNotificationForUser(userID, sender, subject string) {
	val, _ := mailBuffer.LoadOrStore(userID, &mailBucket{})
	buf := val.(*mailBucket)
	buf.mu.Lock()
	buf.items = append(buf.items, pendingMail{
		sender:  sender,
		subject: subject,
	})
	buf.mu.Unlock()
}

func startMailBatcher(app core.App) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		flushMailBuffer(app)
	}
}

func flushMailBuffer(app core.App) {
	mailBuffer.Range(func(key, value any) bool {
		userID := key.(string)
		buf := value.(*mailBucket)

		buf.mu.Lock()
		items := buf.items
		buf.items = nil
		buf.mu.Unlock()

		mailBuffer.Delete(userID)

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

		if len(items) == 1 {
			notify.NotifyUser(app, notify.NotifyParams{
				UserID:  userID,
				Type:    "mail_new_message",
				Package: "mail",
				Title:   fmt.Sprintf("New mail from %s", items[0].sender),
				Body:    items[0].subject,
				URL:     mailNotificationURL,
			})
		} else {
			notify.NotifyUser(app, notify.NotifyParams{
				UserID:  userID,
				Type:    "mail_new_message",
				Package: "mail",
				Title:   fmt.Sprintf("You have %d new messages", len(items)),
				Body:    fmt.Sprintf("From %s and others", items[0].sender),
				URL:     mailNotificationURL,
			})
		}

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
			log.Printf("[mail/batcher] failed to save silent notification: %v", err)
		}
	}
	return important
}
