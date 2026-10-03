package mail

import (
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/acme/autocert"
	"tinycld.org/core/mailproto"
)

// StartIMAPServer starts the IMAP listener. The transport (TLS policy, bind,
// serve, shutdown) lives in core/mailproto; mail supplies the session, which is
// the part that speaks mail's schema.
//
// Listen serves on the "imaps" listener the supervisor that holds the public
// ports passed in (bound once, handed down by name), or binds the address
// itself when there is none, and keeps the listener so a drain can stop it
// accepting. mail still terminates TLS itself here — ExternalTLS stays false —
// so this is the own-ports path, not the injected_listeners.go seam a
// different embedding uses.
func StartIMAPServer(app core.App, certManager *autocert.Manager) (func(), error) {
	return mailproto.StartIMAP(app, certManager, mailproto.IMAPOptions{
		NewSession: func(app core.App, _ *imapserver.Conn) imapserver.Session {
			return newIMAPSession(app)
		},
		Listen: acceptingListeners.listen("imaps"),
	})
}
