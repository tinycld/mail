package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/readonly"
)

// imapFetcherManager runs a SINGLE IMAP polling goroutine for the deployment's
// system mail account. The provider + IMAP credentials are system-wide
// (system_settings, configured in /admin), so there is one mailbox to poll, not
// one per org. Each fetched message is dispatched to whichever org owns the
// recipient's domain (dispatchInbound resolves the mailbox — and thus the org —
// from the recipient address). It (re)starts/stops the fetcher when the system
// mail settings change.
//
// Deliberately simple: a single poll loop on a fixed cadence. IMAP IDLE is a
// follow-up; polling fails predictably without saturating the network with
// reconnect loops.
type imapFetcherManager struct {
	app    core.App
	parent context.Context

	mu     sync.Mutex
	cancel context.CancelFunc // non-nil while the fetcher loop is running
}

// globalIMAPManager is set by startIMAPFetchers and consulted by the settings
// record hook so a settings change can trigger a reconcile. nil before
// startup and after shutdown — callers must nil-check.
var globalIMAPManager *imapFetcherManager

// startIMAPFetchers wires the manager into the app lifecycle. Returns a
// shutdown function for OnTerminate.
func startIMAPFetchers(app core.App) func() {
	ctx, cancel := context.WithCancel(context.Background())
	m := &imapFetcherManager{app: app, parent: ctx}
	globalIMAPManager = m
	go m.reconcile()
	return func() {
		cancel()
		m.stop()
		globalIMAPManager = nil
	}
}

// reconcile starts the fetcher when the system provider is self-hosted SMTP in
// "imap" inbound mode (with a host configured), and stops it otherwise. Called
// on startup and whenever mail/system settings change. Idempotent.
func (m *imapFetcherManager) reconcile() {
	cfg := smtpConfigFromSystem(m.app)
	want := cfg.InboundMode == "imap" && cfg.IMAPHost != ""

	m.mu.Lock()
	defer m.mu.Unlock()
	running := m.cancel != nil
	switch {
	case want && !running:
		ctx, cancel := context.WithCancel(m.parent)
		m.cancel = cancel
		go m.run(ctx)
	case !want && running:
		m.cancel()
		m.cancel = nil
	}
}

// onSettingsChanged is called by the settings/system_settings record hooks.
// Rerunning reconcile is cheap, so we don't try to diff what changed.
func (m *imapFetcherManager) onSettingsChanged() {
	go m.reconcile()
}

func (m *imapFetcherManager) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

// run is the single poll loop. It re-reads system settings each tick so rotating
// credentials (and switching off imap mode) takes effect without a restart.
func (m *imapFetcherManager) run(ctx context.Context) {
	for {
		cfg := smtpConfigFromSystem(m.app)
		if cfg.InboundMode != "imap" || cfg.IMAPHost == "" {
			return
		}
		if err := m.tick(ctx, cfg); err != nil && ctx.Err() == nil {
			m.app.Logger().Warn("imap fetcher: tick failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(cfg.IMAPPollInterval):
		}
	}
}

// tick performs one fetch cycle: connect, login, select mailbox, search for
// unseen messages, fetch + dispatch each (by recipient domain → org), then mark
// seen. Returns the first error encountered (the loop logs and retries next tick).
func (m *imapFetcherManager) tick(ctx context.Context, cfg SMTPConfig) error {
	// A read-only server is waiting to be replaced; a message it stores now
	// is lost if the next build rolls back. Waiting leaves it unseen on the
	// remote server, so a later tick (here or in the next build) pulls it.
	if err := readonly.WaitInactive(ctx); err != nil {
		return fmt.Errorf("imap fetcher: wait for read-only to end: %w", err)
	}
	addr := imapAddress(cfg)

	client, err := dialIMAP(ctx, addr, cfg.IMAPUseTLS)
	if err != nil {
		return fmt.Errorf("dial imap %s: %w", addr, err)
	}
	defer client.Close()

	if err := client.Login(cfg.IMAPUsername, cfg.IMAPPassword).Wait(); err != nil {
		return fmt.Errorf("imap login: %w", err)
	}
	defer func() { _ = client.Logout().Wait() }()

	if _, err := client.Select(cfg.IMAPMailbox, nil).Wait(); err != nil {
		return fmt.Errorf("select %q: %w", cfg.IMAPMailbox, err)
	}

	// Search for unseen messages. NotFlag={\Seen} captures everything we
	// haven't ingested yet — the IMAP server is the source of truth for
	// "already pulled" via the \Seen flag we set after a successful store.
	criteria := &imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagSeen}}
	searchData, err := client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return fmt.Errorf("search unseen: %w", err)
	}
	uids := searchData.AllUIDs()
	if len(uids) == 0 {
		return nil
	}

	fetchOpts := &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{{}}, // full body
	}
	uidSet := imap.UIDSetNum(uids...)
	messages, err := client.Fetch(uidSet, fetchOpts).Collect()
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}

	parser := NewSMTPProvider(cfg)

	var successUIDs []imap.UID
	for i, msgBuf := range messages {
		// Read-only can begin mid-batch; the rest stay unseen for a later tick.
		if readonly.Active() {
			break
		}
		body := firstBodySection(msgBuf)
		if len(body) == 0 {
			continue
		}
		msg, err := parser.ParseInbound(body)
		if err != nil {
			m.app.Logger().Warn("imap fetcher: parse failed", "error", err)
			continue
		}
		if err := dispatchInbound(m.app, msg); err != nil {
			addressLog.Warn("imap fetcher: dispatch failed; the next tick retries",
				"messageID", msg.MessageID, "error", err)
			// Do not mark this UID seen — next tick retries.
			continue
		}
		// Map back to the UID we asked for. messages preserves the search
		// order, so the index aligns with uids[i].
		if i < len(uids) {
			successUIDs = append(successUIDs, uids[i])
		}
	}

	if len(successUIDs) == 0 {
		return nil
	}

	flagOps := &imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagSeen},
	}
	if err := client.Store(imap.UIDSetNum(successUIDs...), flagOps, nil).Close(); err != nil {
		return fmt.Errorf("mark seen: %w", err)
	}
	return nil
}

// imapAddress builds "host:port", defaulting the port by TLS mode.
func imapAddress(cfg SMTPConfig) string {
	port := cfg.IMAPPort
	if port == 0 {
		if cfg.IMAPUseTLS {
			port = 993
		} else {
			port = 143
		}
	}
	return fmt.Sprintf("%s:%d", cfg.IMAPHost, port)
}

// imapDialer is the dial entry point used by the fetcher. Swappable for tests.
var imapDialer = realIMAPDial

// dialIMAP is a thin wrapper that calls through the swappable dialer.
func dialIMAP(ctx context.Context, addr string, useTLS bool) (*imapclient.Client, error) {
	return imapDialer(ctx, addr, useTLS)
}

func realIMAPDial(ctx context.Context, addr string, useTLS bool) (*imapclient.Client, error) {
	if useTLS {
		dialer := net.Dialer{Timeout: 30 * time.Second}
		conn, err := tls.DialWithDialer(&dialer, "tcp", addr, &tls.Config{MinVersion: tls.VersionTLS12})
		if err != nil {
			return nil, err
		}
		return imapclient.New(conn, nil), nil
	}
	d := net.Dialer{Timeout: 30 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	return imapclient.New(conn, nil), nil
}

// dispatchInbound routes a parsed message to each recipient's mailbox. The
// system IMAP account receives mail for every served domain, so routing is
// purely by recipient address: deliveryTargets finds the inboxes via the
// address's own domain (and thus the owning org), which keeps org isolation
// intact without a separate org filter. A group address reaches each member's
// own inbox, and one targetSet stores one copy per inbox. Reuses processInboundForMailbox (same path the webhook +
// inbound SMTP listener use), so threading, FTS, notifications and dedup behave
// identically.
func dispatchInbound(app core.App, msg *InboundMessage) error {
	var dispatched int
	var storeErr, lookupErr error

	var delivered targetSet

	for _, email := range routingRecipients(msg) {
		localPart, domain := splitAddress(email)
		if localPart == "" || domain == "" {
			addressLog.Info("imap fetcher: skipped recipient",
				"address", email, "reason", "malformed address", "messageID", msg.MessageID)
			continue
		}
		localPart = stripPlusTag(localPart)

		targets, err := deliveryTargets(app, localPart, domain)
		if errors.Is(err, errAddressNotFound) {
			addressLog.Info("imap fetcher: skipped recipient",
				"address", localPart+"@"+domain, "reason", err.Error(), "messageID", msg.MessageID)
			continue
		}
		// A failed lookup leaves the message unseen even when another
		// recipient was stored (see the retry rule below).
		if err != nil {
			lookupErr = errors.Join(lookupErr, fmt.Errorf("look up %s: %w", email, err))
			continue
		}
		for _, target := range targets {
			if !delivered.add(target) {
				continue
			}
			if err := processInboundForMailbox(app, target.Mailbox, msg, target.AddressID); err != nil {
				addressLog.Error("imap fetcher: storage failed",
					"mailboxID", target.Mailbox.Id, "address", email, "messageID", msg.MessageID, "error", err)
				storeErr = errors.Join(storeErr, fmt.Errorf("store for %s in inbox %s: %w", email, target.Mailbox.Id, err))
				continue
			}
			dispatched++
		}
	}

	// A failed lookup or store leaves the UID unseen so the next tick
	// retries, which stores only the missing copies (see retryIsSafe).
	// Without a Message-ID the retry would duplicate the copies already
	// stored, so once anything is stored the message is kept as seen and the
	// lost copies are logged.
	failed := errors.Join(lookupErr, storeErr)
	if failed == nil {
		return nil
	}
	if dispatched == 0 || retryIsSafe(msg) {
		return failed
	}
	addressLog.Error("imap fetcher: kept a partial delivery; with no Message-ID a retry would store duplicates",
		"stored", dispatched, "error", failed)
	return nil
}

// firstBodySection returns the first non-empty body section of a fetched
// message. We requested a single empty-section item (full body), so there
// should be exactly one — but we tolerate a couple to keep this robust to
// server quirks.
func firstBodySection(buf *imapclient.FetchMessageBuffer) []byte {
	for _, bs := range buf.BodySection {
		if len(bs.Bytes) > 0 {
			return bs.Bytes
		}
	}
	return nil
}
