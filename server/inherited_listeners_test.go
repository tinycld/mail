package mail

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tinycld.org/core/listeners"
)

// The inherited-listener seam these tests pin: when the supervisor that holds
// the public ports hands this process pre-bound TCP listeners by name ("imaps",
// "submissions", "smtp"), StartIMAPServer/StartSMTPServer/
// StartSMTPInboundServer serve on exactly those listeners instead of binding
// their own — mail still terminates TLS itself, unlike the separate
// injected_listeners.go seam a different embedding uses. With no inherited
// listener for a name, each one binds its configured address as before.

// writeCertPair generates a self-signed cert/key pair and writes them as PEM
// files, mirroring core/server/mailproto/external_tls_test.go's helper (mail
// has no equivalent of its own).
func writeCertPair(t *testing.T, certPath, keyPath, commonName string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

func setupTestCert(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	writeCertPair(t, certPath, keyPath, "mail.test.example")
	return certPath, keyPath
}

func dialAndReadLine(t *testing.T, ln net.Listener, tlsDial bool) string {
	t.Helper()
	var conn net.Conn
	var err error
	if tlsDial {
		conn, err = tls.Dial("tcp", ln.Addr().String(), &tls.Config{InsecureSkipVerify: true})
	} else {
		conn, err = net.Dial("tcp", ln.Addr().String())
	}
	if err != nil {
		t.Fatalf("dial listener: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read greeting: %v", err)
	}
	return line
}

func readEHLOResponse(t *testing.T, ln net.Listener) (greeting, ehlo string) {
	t.Helper()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial listener: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	r := bufio.NewReader(conn)

	greeting, err = r.ReadString('\n')
	if err != nil {
		t.Fatalf("read greeting: %v", err)
	}

	if _, err := conn.Write([]byte("EHLO relay.example\r\n")); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read EHLO response: %v (got so far: %q)", err, b.String())
		}
		b.WriteString(line)
		if !strings.HasPrefix(line, "250-") {
			break
		}
	}
	return greeting, b.String()
}

// TestStartIMAPServer_UsesInheritedListener confirms the production (TLS-only)
// path serves IMAPS on the supervisor's "imaps" listener instead of binding
// :993 itself, and that a client completes a real TLS handshake and reads the
// IMAP greeting over it.
func TestStartIMAPServer_UsesInheritedListener(t *testing.T) {
	certPath, keyPath := setupTestCert(t)
	t.Setenv("IMAP_TLS_CERT", certPath)
	t.Setenv("IMAP_TLS_KEY", keyPath)
	// A real :993 bind would need root; point production at an address this
	// test is never allowed to actually bind, so the only way the test can
	// pass is by serving on the inherited listener below.
	t.Setenv("IMAPS_ADDR", "127.0.0.1:1")

	imapsLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	restore := listeners.SetForTest(map[string]net.Listener{"imaps": imapsLn})
	t.Cleanup(restore)

	shutdown, err := StartIMAPServer(newProdApp(t), nil)
	if err != nil {
		t.Fatalf("StartIMAPServer: %v", err)
	}
	t.Cleanup(shutdown)

	line := dialAndReadLine(t, imapsLn, true)
	if !strings.HasPrefix(line, "* OK") {
		t.Fatalf("IMAP greeting over inherited TLS listener = %q", line)
	}
}

// TestStartIMAPServer_NoInheritedListener_BindsConfiguredAddr confirms that
// with nothing inherited, StartIMAPServer still binds IMAPS_ADDR itself, as it
// did before this seam existed.
func TestStartIMAPServer_NoInheritedListener_BindsConfiguredAddr(t *testing.T) {
	certPath, keyPath := setupTestCert(t)
	t.Setenv("IMAP_TLS_CERT", certPath)
	t.Setenv("IMAP_TLS_KEY", keyPath)
	t.Setenv("IMAPS_ADDR", "127.0.0.1:0")

	restore := listeners.SetForTest(nil)
	t.Cleanup(restore)

	shutdown, err := StartIMAPServer(newProdApp(t), nil)
	if err != nil {
		t.Fatalf("StartIMAPServer: %v", err)
	}
	t.Cleanup(shutdown)
	// A nil-restore leaves no inherited listener to dial; the real assertion
	// is simply that StartIMAPServer did not error and bound its own
	// listener somewhere, which it only could have done via net.Listen.
}

// TestStartSMTPServer_UsesInheritedListener mirrors the IMAP case for the
// submission server's "submissions" listener.
func TestStartSMTPServer_UsesInheritedListener(t *testing.T) {
	certPath, keyPath := setupTestCert(t)
	t.Setenv("SMTP_TLS_CERT", certPath)
	t.Setenv("SMTP_TLS_KEY", keyPath)
	t.Setenv("SMTPS_ADDR", "127.0.0.1:1")

	subLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	restore := listeners.SetForTest(map[string]net.Listener{"submissions": subLn})
	t.Cleanup(restore)

	shutdown, err := StartSMTPServer(newProdApp(t), nil)
	if err != nil {
		t.Fatalf("StartSMTPServer: %v", err)
	}
	t.Cleanup(shutdown)

	conn, err := tls.Dial("tcp", subLn.Addr().String(), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("TLS dial inherited submission listener: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	greeting, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read greeting: %v", err)
	}
	if !strings.HasPrefix(greeting, "220 ") {
		t.Fatalf("submission greeting over inherited TLS listener = %q", greeting)
	}
}

// TestStartSMTPServer_NoInheritedListener_BindsConfiguredAddr mirrors the IMAP
// no-inherited-listener case.
func TestStartSMTPServer_NoInheritedListener_BindsConfiguredAddr(t *testing.T) {
	certPath, keyPath := setupTestCert(t)
	t.Setenv("SMTP_TLS_CERT", certPath)
	t.Setenv("SMTP_TLS_KEY", keyPath)
	t.Setenv("SMTPS_ADDR", "127.0.0.1:0")

	restore := listeners.SetForTest(nil)
	t.Cleanup(restore)

	shutdown, err := StartSMTPServer(newProdApp(t), nil)
	if err != nil {
		t.Fatalf("StartSMTPServer: %v", err)
	}
	t.Cleanup(shutdown)
}

// TestStartSMTPInboundServer_UsesInheritedListener confirms the inbound-MX
// own-ports path serves on the supervisor's "smtp" listener, with a plain
// greeting and STARTTLS advertised (STARTTLS, not external TLS termination —
// mail still does its own TLS on this path).
func TestStartSMTPInboundServer_UsesInheritedListener(t *testing.T) {
	t.Setenv("MAIL_INBOUND_SMTP_ENABLED", "true")
	certPath, keyPath := setupTestCert(t)
	t.Setenv("SMTP_INBOUND_TLS_CERT", certPath)
	t.Setenv("SMTP_INBOUND_TLS_KEY", keyPath)
	t.Setenv("SMTP_INBOUND_ADDR", "127.0.0.1:1")

	mxLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	restore := listeners.SetForTest(map[string]net.Listener{"smtp": mxLn})
	t.Cleanup(restore)

	shutdown, err := StartSMTPInboundServer(newProdApp(t), nil)
	if err != nil {
		t.Fatalf("StartSMTPInboundServer: %v", err)
	}
	t.Cleanup(shutdown)

	greeting, ehlo := readEHLOResponse(t, mxLn)
	if !strings.HasPrefix(greeting, "220 ") {
		t.Fatalf("inbound MX greeting over inherited listener = %q", greeting)
	}
	if !strings.Contains(ehlo, "STARTTLS") {
		t.Fatalf("inbound MX over inherited listener must still advertise STARTTLS (mail terminates TLS itself), got %q", ehlo)
	}
	if strings.Contains(ehlo, "AUTH") {
		t.Fatalf("inbound MX must not offer AUTH, got %q", ehlo)
	}
}

// TestStartSMTPInboundServer_NoInheritedListener_BindsConfiguredAddr mirrors
// the no-inherited-listener case for the inbound-MX server.
func TestStartSMTPInboundServer_NoInheritedListener_BindsConfiguredAddr(t *testing.T) {
	t.Setenv("MAIL_INBOUND_SMTP_ENABLED", "true")
	t.Setenv("SMTP_INBOUND_ADDR", "127.0.0.1:0")

	restore := listeners.SetForTest(nil)
	t.Cleanup(restore)

	shutdown, err := StartSMTPInboundServer(newProdApp(t), nil)
	if err != nil {
		t.Fatalf("StartSMTPInboundServer: %v", err)
	}
	t.Cleanup(shutdown)
}
