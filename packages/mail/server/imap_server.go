package mail

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/pocketbase/pocketbase"
)

// StartIMAPServer reads configuration from environment variables, creates an
// IMAP server, starts listening, and returns a shutdown function.
// When TLS is configured, it starts both a plain+STARTTLS listener and an
// implicit TLS listener per RFC 8314.
func StartIMAPServer(app *pocketbase.PocketBase) (func(), error) {
	if os.Getenv("IMAP_ENABLED") == "false" {
		app.Logger().Info("IMAP server disabled via IMAP_ENABLED=false")
		return func() {}, nil
	}

	addr := os.Getenv("IMAP_ADDR")
	if addr == "" {
		if app.IsDev() {
			addr = ":1143"
		} else {
			addr = ":143"
		}
	}

	insecureAuth := os.Getenv("IMAP_INSECURE_AUTH") == "true" || app.IsDev()

	tlsConfig, err := loadTLSConfig("IMAP_TLS_CERT", "IMAP_TLS_KEY", "", "")
	if err != nil {
		return nil, err
	}

	server := imapserver.New(&imapserver.Options{
		NewSession: func(conn *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			sess := newIMAPSession(app)
			return sess, nil, nil
		},
		Caps: imap.CapSet{
			imap.CapIMAP4rev1:   {},
			imap.CapIMAP4rev2:   {},
			imap.CapSpecialUse:  {},
			imap.CapMove:        {},
			imap.CapIdle:        {},
			imap.CapLiteralPlus: {},
		},
		TLSConfig:    tlsConfig,
		InsecureAuth: insecureAuth,
	})

	// Plain listener (supports STARTTLS when TLS is configured)
	plainLn, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	app.Logger().Info("IMAP server listening", "addr", addr, "starttls", tlsConfig != nil)
	go func() {
		if err := server.Serve(plainLn); err != nil {
			app.Logger().Error("IMAP server error", "addr", addr, "error", err)
		}
	}()

	var tlsLn net.Listener
	if tlsConfig != nil {
		imapsAddr := os.Getenv("IMAPS_ADDR")
		if imapsAddr == "" {
			if app.IsDev() {
				imapsAddr = ":1993"
			} else {
				imapsAddr = ":993"
			}
		}
		tlsLn, err = tls.Listen("tcp", imapsAddr, tlsConfig)
		if err != nil {
			plainLn.Close()
			return nil, fmt.Errorf("failed to listen on %s: %w", imapsAddr, err)
		}
		app.Logger().Info("IMAPS server listening (implicit TLS)", "addr", imapsAddr)
		go func() {
			if err := server.Serve(tlsLn); err != nil {
				app.Logger().Error("IMAPS server error", "addr", imapsAddr, "error", err)
			}
		}()
	}

	shutdown := func() {
		app.Logger().Info("Shutting down IMAP server")
		plainLn.Close()
		if tlsLn != nil {
			tlsLn.Close()
		}
		server.Close()
	}

	return shutdown, nil
}
