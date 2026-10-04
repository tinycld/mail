package mail

import (
	"net"
	"os"
	"sync"

	"tinycld.org/core/drainhooks"
	"tinycld.org/core/listeners"
	"tinycld.org/core/mailproto"
)

// acceptingListeners holds every listener the IMAP, submission and inbound
// SMTP servers accept on. Under a supervisor, a drain begins only once the
// next server is ready on the same ports, so from then on new mail
// connections belong to it. Closing these listeners stops this process
// accepting at once while its open sessions finish within the drain; the
// supervisor's own copy of each port stays open for the next server.
var acceptingListeners = &listenerSet{}

type listenerSet struct {
	mu sync.Mutex
	ls []net.Listener
}

// listen returns a mailproto.ListenFunc that serves the supervisor's
// inherited listener for name only when the requested addr is prodAddr — the
// production implicit-TLS address (IMAPS_ADDR/SMTPS_ADDR) — and otherwise
// binds addr itself. It keeps whatever it returns so a drain can close it.
//
// mailproto's dev path (startIMAPDev/startSMTPDev) calls the same Listen
// closure twice under one name: once for the plain dev address and once for
// the optional implicit-TLS dev address. Without the prodAddr guard, an
// inherited listener handed down for the production TLS address would be
// claimed by whichever of those two calls runs first — including the plain
// one, which would then serve a production, TLS-terminated-by-the-supervisor
// listener in plain text. Comparing addr keeps the inherited listener scoped
// to the one address it actually corresponds to; the dev addresses always
// bind themselves, inherited or not.
func (s *listenerSet) listen(name, prodAddr string) mailproto.ListenFunc {
	return func(addr string) (net.Listener, error) {
		var l net.Listener
		var ok bool
		if addr == prodAddr {
			l, ok = listeners.Inherited(name)
		}
		if !ok {
			var err error
			if l, err = net.Listen("tcp", addr); err != nil {
				return nil, err
			}
		}
		s.mu.Lock()
		s.ls = append(s.ls, l)
		s.mu.Unlock()
		return l, nil
	}
}

// envOrDefault resolves the production TLS address the same way
// mailproto.StartIMAP/StartSMTP do internally (env override, else default),
// so the listen() guard above compares against the exact address mailproto
// will request.
func envOrDefault(env, def string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return def
}

func (s *listenerSet) closeAll() {
	s.mu.Lock()
	ls := s.ls
	s.ls = nil
	s.mu.Unlock()
	for _, l := range ls {
		// The servers' own shutdown closes these again later; a second close
		// only reports that it is already closed.
		_ = l.Close()
	}
}

func registerDrainHook() {
	drainhooks.OnBegin("mail-listeners", acceptingListeners.closeAll)
}
