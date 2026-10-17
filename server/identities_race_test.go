package mail

import (
	"fmt"
	"strings"
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

// Two different users who derive the same email name at the same time. A
// shared inbox holds "kimN", so user "kimN" derives "kimN2", the name user
// "kimN2" derives too. Each gets a different name, and neither save fails.
func TestEnsureIdentity_ConcurrentUsersWithTheSameEmailName(t *testing.T) {
	app := newIdentityTestApp(t)
	domain := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "main.example", "verified": true})
	logs := captureLogs(t)

	for round := range 10 {
		name := fmt.Sprintf("kim%d", round)
		saveRealRecord(t, app, "mail_mailboxes", map[string]any{
			"address": name, "domain": domain.Id, "type": "shared", "display_name": "Desk", "name": "Desk",
		})
		first := saveRealRecord(t, app, "users", map[string]any{"email": name + "a@ext.example", "username": name, "name": "Kim", "role": "member"})
		second := saveRealRecord(t, app, "users", map[string]any{"email": name + "b@ext.example", "username": name + "2", "name": "Kim", "role": "member"})

		errs := make([]error, 2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i, user := range []*core.Record{first, second} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, errs[i] = ensureIdentity(app, user)
			}()
		}
		close(start)
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d, user %d: %v", round, i, err)
			}
		}
		names := map[string]bool{}
		for _, user := range []*core.Record{first, second} {
			identity := identityOf(t, app, user.Id)
			if identity == nil {
				t.Fatalf("round %d: %s has no identity", round, user.GetString("username"))
			}
			names[identity.GetString("email_name")] = true
		}
		if len(names) != 2 || !names[name+"2"] {
			t.Fatalf("round %d: email names %v, want %s2 and one other name", round, names, name)
		}
	}
	if strings.Contains(logs.String(), `"level":"ERROR"`) {
		t.Fatalf("an identity save logged an error:\n%s", logs.String())
	}
}
