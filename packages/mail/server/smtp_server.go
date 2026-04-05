package mail

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase"
)

// StartSMTPServer reads configuration from environment variables, creates an
// SMTP submission server, starts listening, and returns a shutdown function.
// When TLS is configured, it starts both a plain+STARTTLS listener and an
// implicit TLS listener per RFC 8314.
func StartSMTPServer(app *pocketbase.PocketBase) (func(), error) {
	if os.Getenv("SMTP_ENABLED") == "false" {
		app.Logger().Info("SMTP server disabled via SMTP_ENABLED=false")
		return func() {}, nil
	}

	addr := os.Getenv("SMTP_ADDR")
	if addr == "" {
		if app.IsDev() {
			addr = ":1587"
		} else {
			addr = ":587"
		}
	}

	domain := os.Getenv("SMTP_DOMAIN")
	if domain == "" {
		domain = "localhost"
	}

	insecureAuth := os.Getenv("SMTP_INSECURE_AUTH") == "true" || app.IsDev()

	tlsConfig, err := loadTLSConfig("SMTP_TLS_CERT", "SMTP_TLS_KEY", "IMAP_TLS_CERT", "IMAP_TLS_KEY")
	if err != nil {
		return nil, err
	}

	backend := &smtpBackend{app: app}
	server := smtp.NewServer(backend)
	server.Addr = addr
	server.Domain = domain
	server.AllowInsecureAuth = insecureAuth
	server.MaxMessageBytes = 25 << 20
	server.EnableSMTPUTF8 = true
	server.ReadTimeout = 60 * time.Second
	server.WriteTimeout = 60 * time.Second
	if tlsConfig != nil {
		server.TLSConfig = tlsConfig
	}

	// Plain listener (supports STARTTLS when TLS is configured)
	plainLn, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	app.Logger().Info("SMTP server listening", "addr", addr, "starttls", tlsConfig != nil)
	go func() {
		if err := server.Serve(plainLn); err != nil {
			app.Logger().Error("SMTP server error", "addr", addr, "error", err)
		}
	}()

	var tlsLn net.Listener
	if tlsConfig != nil {
		smtpsAddr := os.Getenv("SMTPS_ADDR")
		if smtpsAddr == "" {
			if app.IsDev() {
				smtpsAddr = ":1465"
			} else {
				smtpsAddr = ":465"
			}
		}
		tlsLn, err = tls.Listen("tcp", smtpsAddr, tlsConfig)
		if err != nil {
			plainLn.Close()
			return nil, fmt.Errorf("failed to listen on %s: %w", smtpsAddr, err)
		}
		app.Logger().Info("SMTPS server listening (implicit TLS)", "addr", smtpsAddr)
		go func() {
			if err := server.Serve(tlsLn); err != nil {
				app.Logger().Error("SMTPS server error", "addr", smtpsAddr, "error", err)
			}
		}()
	}

	shutdown := func() {
		app.Logger().Info("Shutting down SMTP server")
		plainLn.Close()
		if tlsLn != nil {
			tlsLn.Close()
		}
		server.Close()
	}

	return shutdown, nil
}
