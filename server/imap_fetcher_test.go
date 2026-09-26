package mail

import (
	"bytes"
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

// TestIMAPFetcher_PullsUnseenAndIngests boots an in-memory IMAP server with
// one unseen message addressed to a seeded mailbox, runs one fetcher tick,
// and asserts the message is stored via the shared processInboundForMailbox
// path. A second tick must be a no-op (the message was marked \Seen), which
// proves the dedup contract that protects against duplicate ingestion on
// every poll.
func TestIMAPFetcher_PullsUnseenAndIngests(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "acme.com", "alice", "mb_imap_fetch_001")
	seedMember(t, app, "mb_imap_fetch_001", "user_alice")

	server, addr, cleanup := newMemIMAPServer(t)
	defer cleanup()

	rawMessage := []byte(strings.Join([]string{
		"From: sender@external.example",
		"To: alice@acme.com",
		"Subject: from imap",
		"Date: Mon, 02 Jan 2006 15:04:05 +0000",
		"Message-ID: <imap-fetch-1@example.org>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"hello via imap",
	}, "\r\n"))

	memUser := imapmemserver.NewUser("alice", "secret")
	if err := memUser.Create("INBOX", &imap.CreateOptions{}); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	if _, err := memUser.Append("INBOX", literalReader(rawMessage), &imap.AppendOptions{}); err != nil {
		t.Fatalf("append: %v", err)
	}
	server.AddUser(memUser)

	// Swap the dialer to target our in-memory server. Without this the
	// fetcher would DNS-resolve "imap.test" and (slowly) fail.
	origDialer := imapDialer
	imapDialer = func(_ context.Context, _ string, _ bool) (*imapclient.Client, error) {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			return nil, err
		}
		return imapclient.New(conn, nil), nil
	}
	t.Cleanup(func() { imapDialer = origDialer })

	cfg := SMTPConfig{
		InboundMode:  "imap",
		IMAPHost:     "imap.test", // ignored — dialer redirects
		IMAPPort:     143,
		IMAPUsername: "alice",
		IMAPPassword: "secret",
		IMAPMailbox:  "INBOX",
	}
	mgr := &imapFetcherManager{app: app}

	if err := mgr.tick(context.Background(), cfg); err != nil {
		t.Fatalf("tick: %v", err)
	}

	msgs, _ := app.FindRecordsByFilter("mail_messages", "subject = {:s}", "", 10, 0, map[string]any{"s": "from imap"})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 stored message, got %d", len(msgs))
	}

	if err := mgr.tick(context.Background(), cfg); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	msgs2, _ := app.FindRecordsByFilter("mail_messages", "subject = {:s}", "", 10, 0, map[string]any{"s": "from imap"})
	if len(msgs2) != 1 {
		t.Errorf("expected still 1 message after re-tick (dedup), got %d", len(msgs2))
	}
}

// dispatchInbound routes purely by recipient domain → owning org. The system
// IMAP account receives mail for every served domain, so isolation must come
// from the address: a message addressed to a domain we don't serve is dropped
// (stored nowhere), and one to a served domain lands only in that domain's
// mailbox.
func TestDispatchInbound_RoutesByRecipientDomain(t *testing.T) {
	app := setupInboundTestApp(t)
	seedDomainAndMailbox(t, app, "acme.com", "alice", "mb_iso_001")
	seedMember(t, app, "mb_iso_001", "user_alice")

	// 1. Recipient on an unserved domain → dropped (no mailbox resolves).
	unserved := &InboundMessage{
		From:      Recipient{Email: "sender@external.example"},
		To:        []Recipient{{Email: "nobody@notserved.example"}},
		Subject:   "unserved",
		TextBody:  "should be stored nowhere",
		MessageID: "<unserved-1@example.org>",
		Date:      time.Now().UTC().Format(time.RFC3339),
	}
	_ = dispatchInbound(app, unserved)
	if msgs, _ := app.FindRecordsByFilter("mail_messages", "subject = {:s}", "", 10, 0, map[string]any{"s": "unserved"}); len(msgs) != 0 {
		t.Fatalf("expected 0 messages for an unserved recipient domain, got %d", len(msgs))
	}

	// 2. Recipient on a served domain → delivered to that domain's mailbox.
	served := &InboundMessage{
		From:      Recipient{Email: "sender@external.example"},
		To:        []Recipient{{Email: "alice@acme.com"}},
		Subject:   "served",
		TextBody:  "delivered to acme",
		MessageID: "<served-1@example.org>",
		Date:      time.Now().UTC().Format(time.RFC3339),
	}
	if err := dispatchInbound(app, served); err != nil {
		t.Fatalf("dispatchInbound: %v", err)
	}
	if msgs, _ := app.FindRecordsByFilter("mail_messages", "subject = {:s}", "", 10, 0, map[string]any{"s": "served"}); len(msgs) != 1 {
		t.Fatalf("expected 1 message delivered to the served domain, got %d", len(msgs))
	}
}

// --- helpers ---

// newMemIMAPServer starts an in-memory IMAP server on a random local port
// and returns the server, its bind address, and a cleanup function.
func newMemIMAPServer(t *testing.T) (*imapmemserver.Server, string, func()) {
	t.Helper()
	memServer := imapmemserver.New()
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(_ *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return memServer.NewSession(), &imapserver.GreetingData{PreAuth: false}, nil
		},
		InsecureAuth: true,
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = srv.Serve(ln)
	}()

	return memServer, ln.Addr().String(), func() {
		ln.Close()
		srv.Close()
		wg.Wait()
	}
}

// literalReader wraps a byte slice in an imap.LiteralReader.
type bufLiteralReader struct {
	*bytes.Reader
	size int64
}

func (l *bufLiteralReader) Size() int64 { return l.size }

func literalReader(b []byte) imap.LiteralReader {
	return &bufLiteralReader{Reader: bytes.NewReader(b), size: int64(len(b))}
}

// io.Reader assertion to keep the imports honest.
var _ io.Reader = (*bytes.Reader)(nil)
