package mail

import (
	"fmt"

	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/mailproto"
)

// TenantListeners are the injected mail sockets a hosting tenant serves on.
// The router owns the public ports (:993/:465/:25): it terminates TLS with the
// wildcard cert (a tenant must never hold that key), demuxes each connection
// to an org, and forwards plaintext over private per-org unix sockets — these
// listeners. A nil entry means the router manages no socket for that service
// and the tenant does not start it.
type TenantListeners struct {
	// IMAP serves the mail client protocol (public :993, TLS at the router).
	IMAP mailproto.ListenFunc
	// Submission serves authenticated client sends (public :465, TLS at the
	// router).
	Submission mailproto.ListenFunc
	// InboundMX receives server-to-server delivery relayed by the router's
	// :25 frontend, which routes each transaction by RCPT TO domain.
	InboundMX mailproto.ListenFunc
}

func (l TenantListeners) empty() bool {
	return l.IMAP == nil && l.Submission == nil && l.InboundMX == nil
}

// registerInjectedListeners serves the protocol stack on router-managed mail
// sockets, in external-TLS mode — the tenant half of Register's listener
// branch. With no listeners injected it registers nothing (the org runs no
// mail listeners; a degraded router binds no mail sockets at all).
func registerInjectedListeners(app *pocketbase.PocketBase, listeners TenantListeners) {
	if listeners.empty() {
		return
	}

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		shutdown, err := startTenantMailListeners(app, listeners)
		if err != nil {
			// A tenant holding a router-managed mail socket it cannot serve
			// must fail its boot loudly — the reason travels back through the
			// readiness pipe — rather than come up healthy on HTTP with mail
			// silently absent (the same policy as the host's failLoud path).
			return fmt.Errorf("aborting tenant startup: %w", err)
		}
		app.OnTerminate().BindFunc(func(te *core.TerminateEvent) error {
			shutdown()
			return te.Next()
		})
		return e.Next()
	})
}

// startTenantMailListeners starts every service with an injected listener and
// returns one aggregate shutdown. A failure unwinds the listeners already
// started so a partial mail stack never outlives the error.
func startTenantMailListeners(app core.App, listeners TenantListeners) (func(), error) {
	var shutdowns []func()
	shutdownAll := func() {
		for _, s := range shutdowns {
			s()
		}
	}

	if listeners.IMAP != nil {
		s, err := mailproto.StartIMAP(app, nil, mailproto.IMAPOptions{
			NewSession: func(app core.App, _ *imapserver.Conn) imapserver.Session {
				return newIMAPSession(app)
			},
			Listen:      listeners.IMAP,
			ExternalTLS: true,
		})
		if err != nil {
			shutdownAll()
			return nil, fmt.Errorf("tenant IMAP listener: %w", err)
		}
		shutdowns = append(shutdowns, s)
	}

	if listeners.Submission != nil {
		s, err := mailproto.StartSMTP(app, nil, mailproto.SMTPOptions{
			Backend:        &smtpBackend{app: app},
			Label:          "SMTP",
			TLSAddrEnv:     "SMTPS_ADDR",
			DefaultTLSAddr: ":465",
			Listen:         listeners.Submission,
			ExternalTLS:    true,
		})
		if err != nil {
			shutdownAll()
			return nil, fmt.Errorf("tenant SMTP submission listener: %w", err)
		}
		shutdowns = append(shutdowns, s)
	}

	if listeners.InboundMX != nil {
		s, err := startSMTPInboundOnListener(app, listeners.InboundMX)
		if err != nil {
			shutdownAll()
			return nil, fmt.Errorf("tenant inbound SMTP listener: %w", err)
		}
		shutdowns = append(shutdowns, s)
	}

	return shutdownAll, nil
}
