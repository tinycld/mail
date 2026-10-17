package mail

import (
	"fmt"
	"sync"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// A new user's create hook and a domain-verified hook (ensureIdentities over
// every user) can run ensureIdentity for the same user at the same time, from
// two requests. Each one read "no identity yet" and then saved one, so the
// second save failed on the unique user index and logged an error.
func TestEnsureIdentity_ConcurrentCallsShareOneIdentity(t *testing.T) {
	app := newIdentityTestApp(t)
	saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	for round := range 10 {
		name := fmt.Sprintf("rae%d", round)
		user := saveRealRecord(t, app, "users", map[string]any{"email": name + "@ext.example", "username": name, "name": "Rae", "role": "member"})
		raceEnsureIdentity(t, app, user)
	}
}

func raceEnsureIdentity(t *testing.T, app core.App, user *core.Record) {
	t.Helper()
	const callers = 16
	ids := make([]string, callers)
	errs := make([]error, callers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			identity, err := ensureIdentity(app, user)
			errs[i] = err
			if identity != nil {
				ids[i] = identity.Id
			}
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
		if ids[i] == "" || ids[i] != ids[0] {
			t.Fatalf("callers got identities %v, want one shared identity", ids)
		}
	}
	identities, err := app.FindRecordsByFilter("mail_identities", "user = {:u}", "", 0, 0, map[string]any{"u": user.Id})
	if err != nil || len(identities) != 1 {
		t.Fatalf("user has %d identities (%v), want 1", len(identities), err)
	}
	inboxes, err := findPersonalMailboxesOf(app, user.Id)
	if err != nil || len(inboxes) != 1 {
		t.Fatalf("user owns %d personal inboxes (%v), want 1", len(inboxes), err)
	}
}
