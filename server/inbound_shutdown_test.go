package mail

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/readonly"
)

// startTestInbound serves the inbound MX for app on a loopback port through
// startSMTPInboundOnListener, which drains it at the app's terminate.
func startTestInbound(t *testing.T, app core.App) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := startSMTPInboundOnListener(app, func(string) (net.Listener, error) { return ln, nil }); err != nil {
		t.Fatal(err)
	}
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

// sendTestMessage delivers one message and returns the server's first
// refusal, or its reply to DATA: nil for 250.
func sendTestMessage(addr, rcpt, message string) error {
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
	if _, err := io.WriteString(w, message); err != nil {
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

func storedWithMessageID(t *testing.T, app core.App, messageID string) int {
	t.Helper()
	rows, err := app.FindRecordsByFilter("mail_messages", "message_id = {:id}", "", 0, 0,
		map[string]any{"id": messageID})
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

// A message the server is storing when the app terminates is stored in full
// before the database closes, and its sender gets the 250. Data reads the
// message to its end, so the drain treats the store as a write and does not
// cut it.
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
	addr := startTestInbound(t, s.app)

	replied := make(chan error, 1)
	go func() {
		replied <- sendTestMessage(addr, "ann@q.example", testInboundMessage("<shutdown-save@sender.example>"))
	}()
	select {
	case <-saving:
	case err := <-replied:
		t.Fatalf("the message was answered before it was stored: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the message never reached its save")
	}

	terminated := terminateApp(s.app)
	// The terminate must not end while the save is held; give a wrong
	// close the chance to happen first.
	select {
	case <-terminated:
		t.Fatal("the terminate closed the database while a DATA save was in progress")
	case <-time.After(500 * time.Millisecond):
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

// inboundMessageOfSize is a message of exactly size bytes as go-smtp counts
// them: the content before the final ".".
func inboundMessageOfSize(messageID string, size int) string {
	header := testInboundMessage(messageID)
	header = strings.TrimSuffix(header, "body\r\n")
	line := strings.Repeat("x", 76) + "\r\n"
	var b strings.Builder
	b.WriteString(header)
	for b.Len()+len(line) <= size-2 {
		b.WriteString(line)
	}
	b.WriteString(strings.Repeat("y", size-2-b.Len()))
	b.WriteString("\r\n")
	return b.String()
}

// A message at or over the size limit is refused with 552 and nothing is
// stored. Reading only up to the limit stored such a message cut short, and
// answered 250.
func TestInboundSMTP_MessageAtSizeLimitRefused(t *testing.T) {
	s := seedGroupDelivery(t)
	addr := startTestInbound(t, s.app)

	for _, size := range []int{maxInboundMessageBytes, maxInboundMessageBytes + 1000} {
		messageID := fmt.Sprintf("<size-%d@sender.example>", size)
		message := inboundMessageOfSize(messageID, size)
		if len(message) != size {
			t.Fatalf("built a %d-byte message, want %d", len(message), size)
		}
		var refusal *smtp.SMTPError
		err := sendTestMessage(addr, "ann@q.example", message)
		if !errors.As(err, &refusal) || refusal.Code != 552 {
			t.Fatalf("%d-byte message: DATA reply = %v, want 552", size, err)
		}
		if n := storedWithMessageID(t, s.app, messageID); n != 0 {
			t.Fatalf("%d-byte message: %d copies stored, want none", size, n)
		}
	}
}

// The read-only refusal reaches the sending MTA through the drained server:
// 451 4.3.2 at MAIL FROM, nothing stored, and delivery works once the mode is
// left.
func TestInboundSMTP_ReadOnlyRefusedThroughServer(t *testing.T) {
	s := seedGroupDelivery(t)
	t.Cleanup(func() { readonly.Leave(s.app) })
	addr := startTestInbound(t, s.app)
	const messageID = "<readonly-inbound@sender.example>"

	readonly.Enter(s.app)
	var refusal *smtp.SMTPError
	err := sendTestMessage(addr, "ann@q.example", testInboundMessage(messageID))
	if !errors.As(err, &refusal) || refusal.Code != 451 || refusal.EnhancedCode != (smtp.EnhancedCode{4, 3, 2}) {
		t.Fatalf("MAIL FROM while read-only = %v, want 451 4.3.2", err)
	}
	if n := storedWithMessageID(t, s.app, messageID); n != 0 {
		t.Fatalf("%d copies stored while read-only, want none", n)
	}

	readonly.Leave(s.app)
	if err := sendTestMessage(addr, "ann@q.example", testInboundMessage(messageID)); err != nil {
		t.Fatalf("delivery after the mode was left: %v", err)
	}
	if n := storedWithMessageID(t, s.app, messageID); n != 1 {
		t.Fatalf("%d copies stored after the mode was left, want 1", n)
	}
}
