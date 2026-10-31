package mail

import (
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/appstate"
	"tinycld.org/core/mailproto"
)

// idleNotifierKey holds the app's IMAP IDLE pub/sub. Mailbox ids are unique
// only in their app, so each app has its own notifier: a change in one app
// must never wake another app's IDLE sessions.
var idleNotifierKey = appstate.NewKey[*mailproto.IdleNotifier]("mail.idleNotifier")

func idleNotifier(app core.App) *mailproto.IdleNotifier {
	return idleNotifierKey.GetOrInit(app, mailproto.NewIdleNotifier)
}
