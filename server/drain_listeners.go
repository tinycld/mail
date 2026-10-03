package mail

import (
	"net"
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

// listen returns a mailproto.ListenFunc that serves on the listener the
// supervisor passed under name, or binds addr when there is none, and keeps
// what it returns so a drain can close it.
func (s *listenerSet) listen(name string) mailproto.ListenFunc {
	return func(addr string) (net.Listener, error) {
		l, ok := listeners.Inherited(name)
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
