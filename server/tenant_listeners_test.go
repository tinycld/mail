package mail

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/rlstest"
)

// The hosting tenant seam these tests pin: when the router hands a tenant
// per-org mail sockets, mail serves the real IMAP / submission / inbound-MX
// sessions on exactly those listeners in external-TLS mode (the router holds
// the wildcard cert and forwards plaintext), and a listener that cannot start
// fails the boot loudly instead of leaving the org quietly mail-less.

func dialMailListener(t *testing.T, ln net.Listener) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial injected listener: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	return conn, bufio.NewReader(conn)
}

func readEHLO(t *testing.T, conn net.Conn, r *bufio.Reader) string {
	t.Helper()
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
			return b.String()
		}
	}
}

func TestStartTenantMailListeners_ServesAllThreeInjectedListeners(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	mkListener := func() net.Listener {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		return ln
	}
	imapLn, subLn, mxLn := mkListener(), mkListener(), mkListener()

	shutdown, err := startTenantMailListeners(app, TenantListeners{
		IMAP:       func(string) (net.Listener, error) { return imapLn, nil },
		Submission: func(string) (net.Listener, error) { return subLn, nil },
		InboundMX:  func(string) (net.Listener, error) { return mxLn, nil },
	})
	if err != nil {
		t.Fatalf("startTenantMailListeners: %v", err)
	}
	t.Cleanup(shutdown)

	// IMAP: plaintext greeting from the real mail session.
	_, imapR := dialMailListener(t, imapLn)
	if greeting, err := imapR.ReadString('\n'); err != nil || !strings.HasPrefix(greeting, "* OK") {
		t.Fatalf("IMAP greeting = %q, err=%v", greeting, err)
	}

	// Submission: auth offered over plaintext (router terminated TLS), no STARTTLS.
	subConn, subR := dialMailListener(t, subLn)
	if greeting, err := subR.ReadString('\n'); err != nil || !strings.HasPrefix(greeting, "220 ") {
		t.Fatalf("submission greeting = %q, err=%v", greeting, err)
	}
	subEhlo := readEHLO(t, subConn, subR)
	if !strings.Contains(subEhlo, "AUTH") {
		t.Fatalf("submission must offer AUTH over the injected listener, got %q", subEhlo)
	}
	if strings.Contains(subEhlo, "STARTTLS") {
		t.Fatalf("submission must not advertise STARTTLS (router terminates), got %q", subEhlo)
	}

	// Inbound MX: server-to-server, never offers AUTH or STARTTLS.
	mxConn, mxR := dialMailListener(t, mxLn)
	if greeting, err := mxR.ReadString('\n'); err != nil || !strings.HasPrefix(greeting, "220 ") {
		t.Fatalf("inbound MX greeting = %q, err=%v", greeting, err)
	}
	mxEhlo := readEHLO(t, mxConn, mxR)
	if strings.Contains(mxEhlo, "AUTH") {
		t.Fatalf("inbound MX must not offer AUTH, got %q", mxEhlo)
	}
	if strings.Contains(mxEhlo, "STARTTLS") {
		t.Fatalf("inbound MX must not advertise STARTTLS (router terminates), got %q", mxEhlo)
	}
}

func TestStartTenantMailListeners_FailureIsLoudAndUnwindsPriorListeners(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	imapLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	_, err = startTenantMailListeners(app, TenantListeners{
		IMAP:       func(string) (net.Listener, error) { return imapLn, nil },
		Submission: func(string) (net.Listener, error) { return nil, fmt.Errorf("socket gone") },
	})
	if err == nil {
		t.Fatal("a mail listener that cannot start must fail the tenant boot, not proceed silently")
	}

	// The already-started IMAP listener must have been unwound: accepting is
	// over, so a fresh dial gets no greeting.
	conn, dialErr := net.Dial("tcp", imapLn.Addr().String())
	if dialErr == nil {
		t.Cleanup(func() { conn.Close() })
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if line, readErr := bufio.NewReader(conn).ReadString('\n'); readErr == nil {
			t.Fatalf("IMAP listener still serving after failed start (greeting %q)", line)
		}
	}
}

// With router-managed listeners injected, the tenant composition matches the
// host exactly. Composed directly from Register's two halves (registerShared +
// registerInjectedListeners) because the tenant context that selects the
// branch is stamped by coreserver.RegisterTenant, outside this package; the
// detection branch itself is covered end-to-end by the orgmanager mail e2e.
func TestTenantWithListenersCompositionMatchesHostExactly(t *testing.T) {
	host := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	Register(host)

	tenant := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	registerShared(tenant)
	registerInjectedListeners(tenant, TenantListeners{
		IMAP: func(string) (net.Listener, error) { return nil, fmt.Errorf("never served in this test") },
	})

	rlstest.AssertCompositionDiff(t,
		rlstest.HookHandlerCounts(t, host),
		rlstest.HookHandlerCounts(t, tenant),
		map[string]int{})
}
