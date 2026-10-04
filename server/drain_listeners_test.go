package mail

import (
	"bufio"
	"crypto/tls"
	"net"
	"strings"
	"testing"
	"time"

	"tinycld.org/core/drainhooks"
	"tinycld.org/core/listeners"
)

// When a supervised server begins to drain, the next server is already
// accepting on the same ports. Mail must stop accepting at once, not when its
// terminate hook runs after the HTTP drain: an inbound message this process
// answers 250 to after the backup is lost if the next build rolls back.
// Sessions already open may still finish.

func refused(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return true
	}
	c.Close()
	return false
}

func TestDrainBeginStopsInboundAcceptingAndKeepsSessions(t *testing.T) {
	t.Cleanup(drainhooks.ResetForTest)
	registerDrainHook()
	t.Setenv("MAIL_INBOUND_SMTP_ENABLED", "true")
	t.Setenv("SMTP_INBOUND_ADDR", "127.0.0.1:1")

	mxLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(listeners.SetForTest(map[string]net.Listener{"smtp": mxLn}))
	shutdown, err := StartSMTPInboundServer(newProdApp(t), nil)
	if err != nil {
		t.Fatalf("StartSMTPInboundServer: %v", err)
	}
	t.Cleanup(shutdown)

	conn, err := net.Dial("tcp", mxLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	r := bufio.NewReader(conn)
	if greeting, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(greeting, "220 ") {
		t.Fatalf("greeting = %q, %v", greeting, err)
	}

	drainhooks.RunBegin()

	if !refused(mxLn.Addr().String()) {
		t.Fatal("the inbound server still accepts after the drain began")
	}
	if _, err := conn.Write([]byte("NOOP\r\n")); err != nil {
		t.Fatal(err)
	}
	if line, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(line, "250") {
		t.Fatalf("an open session was cut by the drain: %q, %v", line, err)
	}
}

func TestDrainBeginStopsIMAPAndSubmissionAccepting(t *testing.T) {
	t.Cleanup(drainhooks.ResetForTest)
	registerDrainHook()
	certPath, keyPath := setupTestCert(t)
	t.Setenv("IMAP_TLS_CERT", certPath)
	t.Setenv("IMAP_TLS_KEY", keyPath)
	t.Setenv("SMTP_TLS_CERT", certPath)
	t.Setenv("SMTP_TLS_KEY", keyPath)
	t.Setenv("IMAPS_ADDR", "127.0.0.1:1")
	t.Setenv("SMTPS_ADDR", "127.0.0.1:1")

	imapsLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	subLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(listeners.SetForTest(map[string]net.Listener{"imaps": imapsLn, "submissions": subLn}))
	app := newProdApp(t)
	imapShutdown, err := StartIMAPServer(app, nil)
	if err != nil {
		t.Fatalf("StartIMAPServer: %v", err)
	}
	t.Cleanup(imapShutdown)
	smtpShutdown, err := StartSMTPServer(app, nil)
	if err != nil {
		t.Fatalf("StartSMTPServer: %v", err)
	}
	t.Cleanup(smtpShutdown)

	imapConn, err := tls.Dial("tcp", imapsLn.Addr().String(), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { imapConn.Close() })
	_ = imapConn.SetDeadline(time.Now().Add(5 * time.Second))
	imapR := bufio.NewReader(imapConn)
	if greeting, err := imapR.ReadString('\n'); err != nil || !strings.HasPrefix(greeting, "* OK") {
		t.Fatalf("IMAP greeting = %q, %v", greeting, err)
	}

	drainhooks.RunBegin()

	if !refused(imapsLn.Addr().String()) {
		t.Fatal("the IMAP server still accepts after the drain began")
	}
	if !refused(subLn.Addr().String()) {
		t.Fatal("the submission server still accepts after the drain began")
	}
	if _, err := imapConn.Write([]byte("a1 NOOP\r\n")); err != nil {
		t.Fatal(err)
	}
	if line, err := imapR.ReadString('\n'); err != nil || !strings.HasPrefix(line, "a1 OK") {
		t.Fatalf("an open IMAP session was cut by the drain: %q, %v", line, err)
	}
}
