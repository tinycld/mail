package mail

import (
	"fmt"

	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/mailproto"
)

// InjectedListeners are pre-bound mail sockets handed down by a supervisor
// that owns the public ports (:993/:465/:25): it terminates TLS with a
// certificate this process must never hold, and forwards plaintext over
// private sockets — these listeners. A nil entry means the supervisor manages
// no socket for that service and this process does not start it.
type InjectedListeners struct {
	// IMAP serves the mail client protocol (public :993, TLS at the supervisor).
	IMAP mailproto.ListenFunc
	// Submission serves authenticated client sends (public :465, TLS at the
	// supervisor).
	Submission mailproto.ListenFunc
	// InboundMX receives server-to-server delivery relayed by the supervisor's
	// :25 frontend, which routes each transaction by RCPT TO domain.
	InboundMX mailproto.ListenFunc
}

func (l InjectedListeners) empty() bool {
	return l.IMAP == nil && l.Submission == nil && l.InboundMX == nil
}

// registerInjectedListeners serves the protocol stack on supervisor-managed
// mail sockets, in external-TLS mode — the injected half of Register's
// listener branch. With no listeners injected it registers nothing (this
// deployment runs no mail listeners at all).
func registerInjectedListeners(app *pocketbase.PocketBase, listeners InjectedListeners) {
	if listeners.empty() {
		return
	}

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		shutdown, err := startInjectedMailListeners(app, listeners)
		if err != nil {
			// A process holding a supervisor-managed mail socket it cannot serve
			// must fail its boot loudly — the reason travels back through the
			// readiness pipe — rather than come up healthy on HTTP with mail
			// silently absent (the same policy as the host's failLoud path).
			return fmt.Errorf("aborting startup: %w", err)
		}
		app.OnTerminate().BindFunc(func(te *core.TerminateEvent) error {
			shutdown()
			return te.Next()
		})
		return e.Next()
	})
}

// startInjectedMailListeners starts every service with an injected listener and
// returns one aggregate shutdown. A failure unwinds the listeners already
// started so a partial mail stack never outlives the error.
func startInjectedMailListeners(app core.App, listeners InjectedListeners) (func(), error) {
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
			return nil, fmt.Errorf("injected IMAP listener: %w", err)
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
			return nil, fmt.Errorf("injected SMTP submission listener: %w", err)
		}
		shutdowns = append(shutdowns, s)
	}

	if listeners.InboundMX != nil {
		s, err := startSMTPInboundOnListener(app, listeners.InboundMX)
		if err != nil {
			shutdownAll()
			return nil, fmt.Errorf("injected inbound SMTP listener: %w", err)
		}
		shutdowns = append(shutdowns, s)
	}

	return shutdownAll, nil
}
