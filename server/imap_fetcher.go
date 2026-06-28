package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/pocketbase/pocketbase/core"
)

// imapFetcherManager runs one IMAP polling goroutine per org that has the
// self-hosted SMTP provider configured in "imap" inbound mode. It listens for
// mail-settings record changes to (re)spawn workers when an org switches
// modes, edits credentials, or removes its config.
//
// We deliberately keep this simple — one fetcher per org, on a per-org poll
// interval. IMAP IDLE is a follow-up; the current design pulls on a fixed
// cadence so a misconfigured account fails predictably without saturating
// the network with reconnect loops.
type imapFetcherManager struct {
	app    core.App
	parent context.Context

	mu      sync.Mutex
	workers map[string]context.CancelFunc // orgID → cancel
}

// globalIMAPManager is set by startIMAPFetchers and consulted by the settings
// record hook so a settings change can trigger a reconcile. nil before
// startup and after shutdown — callers must nil-check.
var globalIMAPManager *imapFetcherManager

// startIMAPFetchers wires the manager into the app lifecycle. Returns a
// shutdown function for OnTerminate.
func startIMAPFetchers(app core.App) func() {
	ctx, cancel := context.WithCancel(context.Background())
	m := &imapFetcherManager{
		app:     app,
		parent:  ctx,
		workers: make(map[string]context.CancelFunc),
	}
	globalIMAPManager = m
	go m.reconcile()
	return func() {
		cancel()
		m.stopAll()
		globalIMAPManager = nil
	}
}

// reconcile sweeps the settings table and spawns/stops workers to match the
// current per-org configuration. Called on startup and (via the caller) any
// time mail settings change. Deliberately tolerant of read errors — we'd
// rather skip a sweep than crash the loop.
func (m *imapFetcherManager) reconcile() {
	records, err := m.app.FindRecordsByFilter(
		"settings",
		"app = 'mail' && key = 'provider' && value = '\"smtp\"'",
		"",
		0,
		0,
	)
	if err != nil {
		m.app.Logger().Warn("imap fetcher: failed to list smtp-provider orgs", "error", err)
		return
	}

	desired := make(map[string]bool)
	for _, rec := range records {
		orgID := rec.GetString("org")
		if orgID == "" {
			continue
		}
		cfg := smtpConfigFromSettings(m.app, getOrgSettings(m.app, "mail", orgID))
		if cfg.InboundMode != "imap" || cfg.IMAPHost == "" {
			continue
		}
		desired[orgID] = true
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop workers for orgs that are no longer configured.
	for orgID, cancel := range m.workers {
		if !desired[orgID] {
			cancel()
			delete(m.workers, orgID)
		}
	}
	// Start workers for newly-configured orgs.
	for orgID := range desired {
		if _, running := m.workers[orgID]; running {
			continue
		}
		ctx, cancel := context.WithCancel(m.parent)
		m.workers[orgID] = cancel
		go m.runOrg(ctx, orgID)
	}
}

// onSettingsChanged is called by the settings record hooks. We rate-limit by
// rerunning reconcile (cheap — bounded by the org count) instead of trying
// to figure out exactly what changed.
func (m *imapFetcherManager) onSettingsChanged() {
	go m.reconcile()
}

func (m *imapFetcherManager) stopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, cancel := range m.workers {
		cancel()
	}
	m.workers = nil
}

// runOrg is the per-org polling loop. It re-reads settings each tick so
// rotating credentials takes effect without restart.
func (m *imapFetcherManager) runOrg(ctx context.Context, orgID string) {
	for {
		cfg := smtpConfigFromSettings(m.app, getOrgSettings(m.app, "mail", orgID))
		if cfg.InboundMode != "imap" || cfg.IMAPHost == "" {
			return
		}
		if err := m.tickOrg(ctx, orgID, cfg); err != nil {
			m.app.Logger().Warn("imap fetcher: tick failed", "org", orgID, "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(cfg.IMAPPollInterval):
		}
	}
}

// tickOrg performs one fetch cycle: connect, login, select mailbox, search
// for unseen messages, fetch + ingest each, then mark seen. Returns the first
// error encountered (the loop logs and continues on the next tick).
func (m *imapFetcherManager) tickOrg(ctx context.Context, orgID string, cfg SMTPConfig) error {
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
		body := firstBodySection(msgBuf)
		if len(body) == 0 {
			continue
		}
		msg, err := parser.ParseInbound(body)
		if err != nil {
			m.app.Logger().Warn("imap fetcher: parse failed", "org", orgID, "error", err)
			continue
		}
		if err := dispatchToOrgMailboxes(m.app, orgID, msg); err != nil {
			m.app.Logger().Warn("imap fetcher: dispatch failed",
				"org", orgID, "messageID", msg.MessageID, "error", err)
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

// dispatchToOrgMailboxes routes a parsed message to every recipient mailbox
// that belongs to the given org. Reuses processInboundForMailbox (same path
// the webhook + inbound SMTP listener use), so threading, FTS, notifications
// and dedup all behave identically.
func dispatchToOrgMailboxes(app core.App, orgID string, msg *InboundMessage) error {
	var dispatched int
	var firstErr error

	all := make([]Recipient, 0, len(msg.To)+len(msg.Cc))
	all = append(all, msg.To...)
	all = append(all, msg.Cc...)

	for _, rcpt := range all {
		localPart, domain := splitAddress(rcpt.Email)
		if localPart == "" || domain == "" {
			continue
		}
		localPart = stripPlusTag(localPart)

		mailbox, _, err := resolveMailboxByAddress(app, localPart, domain)
		if err != nil {
			continue
		}
		// Confirm the resolved mailbox belongs to this org — the IMAP account
		// could (in principle) deliver mail addressed to a domain on a
		// different org we happen to also host. Org isolation is critical.
		mailboxDomain, err := app.FindRecordById("mail_domains", mailbox.GetString("domain"))
		if err != nil || mailboxDomain.GetString("org") != orgID {
			continue
		}
		if err := processInboundForMailbox(app, mailbox, msg); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		dispatched++
	}

	if dispatched == 0 && firstErr != nil {
		return firstErr
	}
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
