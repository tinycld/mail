package mail

import (
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/crypto/acme/autocert"
	"tinycld.org/core/mailproto"
)

// defaultSMTPSAddr is the production submission address when SMTPS_ADDR is
// unset. mailproto receives it as DefaultTLSAddr and the inherited-listener
// guard compares against it, so both read this one constant.
const defaultSMTPSAddr = ":465"

// StartSMTPServer starts the authenticated SMTP submission listener. The
// transport lives in core/mailproto; mail supplies the backend.
//
// SMTP falls back to the IMAP_TLS_* pair, so a missing SMTP cert is fine as
// long as IMAP's is set.
//
// Listen serves on the supervisor's "submissions" listener, but only for the
// production SMTPS_ADDR request — the dev addresses (:1587, :1465) always
// bind themselves, even under a supervisor; see StartIMAPServer's comment for
// both that guard and the own-ports-vs-injected-listener distinction.
func StartSMTPServer(app core.App, certManager *autocert.Manager) (func(), error) {
	return mailproto.StartSMTP(app, certManager, mailproto.SMTPOptions{
		Backend:           &smtpBackend{app: app},
		Label:             "SMTP",
		EnabledEnv:        "SMTP_ENABLED",
		AddrEnv:           "SMTP_ADDR",
		TLSAddrEnv:        "SMTPS_ADDR",
		DefaultTLSAddr:    defaultSMTPSAddr,
		DefaultAddr:       ":1587",
		DefaultDevTLSAddr: ":1465",
		CertEnv:           "SMTP_TLS_CERT",
		KeyEnv:            "SMTP_TLS_KEY",
		FallbackCertEnv:   "IMAP_TLS_CERT",
		FallbackKeyEnv:    "IMAP_TLS_KEY",
		InsecureAuthEnv:   "SMTP_INSECURE_AUTH",
		ProductionTLSError: "SMTPS (" + defaultSMTPSAddr + ") cannot start: no TLS configured in production. " +
			"Set SMTP_TLS_CERT/SMTP_TLS_KEY (or IMAP_TLS_CERT/IMAP_TLS_KEY) " +
			"to readable cert/key files, or enable autocert " +
			"(AUTOCERT_ENABLED=true + PRIMARY_DOMAIN), " +
			"or set SMTP_ENABLED=false to run without SMTP submission",
		Listen: acceptingListeners.listen("submissions", envOrDefault("SMTPS_ADDR", defaultSMTPSAddr)),
	})
}
