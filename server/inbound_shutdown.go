package mail

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/emersion/go-smtp"
)

// inboundDrainBound bounds how long the app's terminate lets open inbound
// sessions finish before it cuts them. A hung client must not hold up the
// shutdown, and the terminate chain has about 10 s in all before the
// supervisor kills the process (see supervise's stopBound). Under a
// supervisor the listener closed when the drain began, so the sessions have
// had the whole HTTP drain to end and few are left by now.
const inboundDrainBound = 5 * time.Second

// errInboundShuttingDown refuses a new transaction while the server shuts
// down. A 4xx makes the sending MTA queue the message and retry, by then on
// the next server.
var errInboundShuttingDown = &smtp.SMTPError{
	Code:         421,
	EnhancedCode: smtp.EnhancedCode{4, 3, 2},
	Message:      "Mail service is shutting down; try again later",
}

// sessionGate keeps the inbound sessions off the database once the app
// terminates. go-smtp's Shutdown waits for the sessions, but only until its
// context ends; after that it leaves them open, and its Close does nothing
// once Shutdown has run. So the gate keeps its own list of connections to cut
// and counts the session calls that use the database, and the shutdown waits
// for those calls before the database closes.
//
// A nil gate lets every call through: tests build sessions without one.
type sessionGate struct {
	mu sync.Mutex
	// draining refuses new transactions; a transaction already past MAIL
	// FROM may still finish.
	draining bool
	// closed refuses every call that uses the database.
	closed bool
	// conns holds each open connection, and whether its session is storing
	// a message it has read in full.
	conns    map[net.Conn]bool
	inFlight sync.WaitGroup
}

func newSessionGate() *sessionGate {
	return &sessionGate{conns: map[net.Conn]bool{}}
}

func (g *sessionGate) track(c net.Conn) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.conns[c] = false
}

func (g *sessionGate) untrack(c net.Conn) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.conns, c)
}

// setStoring marks whether c's session is storing a message it has read in
// full. cut leaves such a session open: the store runs to the end either way,
// and the open connection carries its reply, so the sending MTA does not
// retry a message that was stored.
func (g *sessionGate) setStoring(c net.Conn, storing bool) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, open := g.conns[c]; open {
		g.conns[c] = storing
	}
}

// enter admits one session call and returns the func that ends it. A call
// that starts a transaction is refused once the drain begins; any other call
// is refused only once the gate is closed.
func (g *sessionGate) enter(startsTransaction bool) (func(), error) {
	if g == nil {
		return func() {}, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || (startsTransaction && g.draining) {
		return nil, errInboundShuttingDown
	}
	g.inFlight.Add(1)
	return g.inFlight.Done, nil
}

// cut closes every open connection but those storing a message. It closes the network connection, not
// the smtp.Conn: the smtp.Conn's Close runs the session's Logout, which must
// run on the session's own goroutine, not beside a Data call in progress. A
// body read in progress fails, and its Data call returns without storing.
func (g *sessionGate) cut() {
	g.mu.Lock()
	conns := make([]net.Conn, 0, len(g.conns))
	for c, storing := range g.conns {
		if !storing {
			conns = append(conns, c)
		}
	}
	g.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

// shutdownInbound stops server and returns only when no session uses the
// database any more:
//
//  1. Stop accepting and refuse new transactions.
//  2. Let the open sessions finish, up to drainBound.
//  3. Cut the sessions still open, but not one storing a message.
//  4. Refuse every later call, and wait for the calls in progress. A Data
//     call that is storing a message runs to the end, with no bound: the
//     database is still open, and a message cut in the middle could be
//     stored in some inboxes and not in others. A Data call still reading
//     its body fails once its connection is cut, and stores nothing.
//
// served closes when the server's Serve loop has returned.
func shutdownInbound(server *smtp.Server, ln net.Listener, gate *sessionGate, served <-chan struct{}, drainBound time.Duration) {
	// Serve can still be about to add ln to the server's list, which
	// Shutdown closes; closed here, Serve returns at once either way.
	_ = ln.Close()
	gate.mu.Lock()
	gate.draining = true
	gate.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), drainBound)
	defer cancel()
	if err := server.Shutdown(ctx); errors.Is(err, context.DeadlineExceeded) {
		addressLog.Warn("inbound SMTP: cutting the sessions still open after the drain", "bound", drainBound)
		gate.cut()
	}

	gate.mu.Lock()
	gate.closed = true
	gate.mu.Unlock()
	gate.inFlight.Wait()
	<-served
}

// serveInbound runs server on ln and returns the func that shuts it down,
// letting open sessions finish for up to drainBound.
func serveInbound(server *smtp.Server, ln net.Listener, gate *sessionGate, addr string, drainBound time.Duration) func() {
	served := make(chan struct{})
	go func() {
		defer close(served)
		err := server.Serve(ln)
		if err != nil && !errors.Is(err, net.ErrClosed) {
			addressLog.Error("inbound SMTP server error", "addr", addr, "error", err)
		}
	}()
	return func() {
		addressLog.Info("shutting down inbound SMTP server", "addr", addr)
		shutdownInbound(server, ln, gate, served, drainBound)
	}
}
