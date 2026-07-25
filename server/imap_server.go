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
func StartIMAPServer(app core.App, certManager *autocert.Manager) (func(), error) {
	return mailproto.StartIMAP(app, certManager,
		func(app core.App, _ *imapserver.Conn) imapserver.Session {
			return newIMAPSession(app)
		},
	)
}
