package mail

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// startTestInbound serves the inbound MX for app on a loopback port and binds
// its shutdown to app's terminate, as registerMailListeners does.
func startTestInbound(t *testing.T, app core.App, drainBound time.Duration) string {
	t.Helper()
	server, gate := newInboundSMTPServer(app, "mx.example", nil)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	bindListenerShutdown(app, serveInbound(server, ln, gate, ln.Addr().String(), drainBound))
	return ln.Addr().String()
}

func testInboundMessage(messageID string) string {
	return fmt.Sprintf("From: Sender <sender@sender.example>\r\n"+
		"To: ann@q.example\r\n"+
		"Subject: Shutdown\r\n"+
		"Message-ID: %s\r\n"+
		"Date: %s\r\n"+
		"\r\n"+
		"body\r\n", messageID, time.Now().UTC().Format(time.RFC1123Z))
}

// sendTestMessage delivers one message and returns the server's reply to
// DATA: nil for 250.
func sendTestMessage(addr, rcpt, messageID string) error {
	c, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Mail("sender@sender.example", nil); err != nil {
		return err
	}
	if err := c.Rcpt(rcpt, nil); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, testInboundMessage(messageID)); err != nil {
		return err
	}
	return w.Close()
}

// terminateApp runs the app's terminate, which closes the database, and
// returns a channel that closes when it has finished. The test's own cleanup
// runs it again later, which returns at once.
func terminateApp(app *tests.TestApp) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		app.Cleanup()
	}()
	return done
}

// A message the server is storing when the app terminates is stored in full
// before the database closes, and its sender gets the 250, even when the
// store outlasts the drain bound.
func TestInboundShutdown_TerminateWaitsForDataSave(t *testing.T) {
	s := seedGroupDelivery(t)
	saving := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	s.app.OnRecordCreate("mail_messages").BindFunc(func(e *core.RecordEvent) error {
		once.Do(func() { close(saving) })
		<-release
		return e.Next()
	})
	const drainBound = 50 * time.Millisecond
	addr := startTestInbound(t, s.app, drainBound)

	replied := make(chan error, 1)
	go func() { replied <- sendTestMessage(addr, "ann@q.example", "<shutdown-save@sender.example>") }()
	select {
	case <-saving:
	case err := <-replied:
		t.Fatalf("the message was answered before it was stored: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the message never reached its save")
	}

	terminated := terminateApp(s.app)
	select {
	case <-terminated:
		t.Fatal("the terminate closed the database while a DATA save was in progress")
	case <-time.After(10 * drainBound):
	}

	close(release)
	select {
	case <-terminated:
	case <-time.After(10 * time.Second):
		t.Fatal("the terminate did not end after the save finished")
	}
	// A save that ran into a closed database fails, and the reply is then a
	// 451, so the 250 shows the save finished before the close.
	if err := <-replied; err != nil {
		t.Fatalf("DATA reply = %v, want 250", err)
	}
}

// A client that stops in the middle of its message, and one that connects
// and sends nothing, do not hold up the terminate beyond the drain bound.
func TestInboundShutdown_StuckSessionsDoNotBlockTerminate(t *testing.T) {
	s := seedGroupDelivery(t)
	const drainBound = 100 * time.Millisecond
	addr := startTestInbound(t, s.app, drainBound)

	idle, err := smtp.Dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idle.Close() })
	if err := idle.Hello("idle.example"); err != nil {
		t.Fatal(err)
	}

	stalled, err := smtp.Dial(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stalled.Close() })
	if err := stalled.Mail("sender@sender.example", nil); err != nil {
		t.Fatal(err)
	}
	if err := stalled.Rcpt("ann@q.example", nil); err != nil {
		t.Fatal(err)
	}
	// After the 354 the server's Data call is reading a body that never
	// ends: the client sends nothing more.
	if _, err := stalled.Data(); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	terminated := terminateApp(s.app)
	select {
	case <-terminated:
	case <-time.After(5 * time.Second):
		t.Fatal("stuck sessions held up the terminate beyond the drain bound")
	}
	if elapsed := time.Since(start); elapsed < drainBound {
		t.Fatalf("the terminate took %v, less than the drain bound %v: the sessions got no drain", elapsed, drainBound)
	}
}
