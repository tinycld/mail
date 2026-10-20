package mail

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/mailproto"
)

type addressFolderEnv struct {
	app      core.App
	user     *core.Record
	mailbox  *core.Record
	identity *core.Record
	// a, b and billing are visible; hidden is in hidden_address_views.
	a, b, billing, hidden *core.Record
	wantA                 int
	wantUnseenA           int
	wantB                 int
}

const (
	addressA      = "owner@example.org"
	addressB      = "sales@example.org"
	addressBill   = "billing@zeta.example"
	addressHidden = "hidden@example.org"
)

// seedAddressFolder builds one address with more threads than PocketBase's
// 200-expression filter limit, plus threads that must NOT appear in its
// folder: inbox threads of another address, another user's threads with the
// same keys, and archived threads with the same keys. The switch starts off.
func seedAddressFolder(t *testing.T) *addressFolderEnv {
	t.Helper()
	app := newRealSchemaApp(t)
	mailbox := seedRealMailbox(t, app)
	mainDomain, err := app.FindRecordById("mail_domains", mailbox.GetString("domain"))
	if err != nil {
		t.Fatal(err)
	}
	mainDomain.Set("rank", 2)
	if err := app.Save(mainDomain); err != nil {
		t.Fatal(err)
	}
	zeta := saveRealRecord(t, app, "mail_domains", map[string]any{"domain": "zeta.example", "rank": 1})

	user := saveRealRecord(t, app, "users", map[string]any{
		"email": "owner@example.org", "username": "owner", "name": "Owner", "role": "member",
	})
	other := saveRealRecord(t, app, "users", map[string]any{
		"email": "other@example.org", "username": "other", "name": "Other", "role": "member",
	})

	address := func(local string, domain *core.Record) *core.Record {
		return saveRealRecord(t, app, "mail_addresses", map[string]any{
			"local_part": local, "domain": domain.Id, "mailbox": mailbox.Id, "source": "manual",
		})
	}
	a := address("owner", mainDomain)
	b := address("sales", mainDomain)
	billing := address("billing", zeta)
	hidden := address("hidden", mainDomain)

	identity := saveRealRecord(t, app, "mail_identities", map[string]any{
		"user": user.Id, "inbox": mailbox.Id, "email_name": "owner",
		"hidden_address_views": []string{hidden.Id},
	})

	env := &addressFolderEnv{
		app: app, user: user, mailbox: mailbox, identity: identity,
		a: a, b: b, billing: billing, hidden: hidden,
	}
	for i := 0; i < 250; i++ {
		owner, folder, keys := user.Id, "inbox", a.Id
		switch {
		case i >= 240:
			folder = "archive"
		case i >= 230:
			owner = other.Id
		case i >= 210:
			keys = b.Id
			env.wantB++
		case i%2 == 0:
			keys = formatAddressKeys([]string{a.Id, b.Id})
			env.wantB++
		}
		isRead := i%3 == 0
		if i < 210 {
			env.wantA++
			if !isRead {
				env.wantUnseenA++
			}
		}
		thread := saveRealRecord(t, app, "mail_threads", map[string]any{
			"mailbox": mailbox.Id, "subject": fmt.Sprintf("thread %d", i),
			"latest_date": "2026-10-01 09:00:00.000Z",
		})
		saveRealRecord(t, app, "mail_thread_state", map[string]any{
			"thread": thread.Id, "user": owner, "folder": folder, "mailbox": mailbox.Id,
			"address_keys": keys, "is_read": isRead,
		})
		saveRealRecord(t, app, "mail_messages", map[string]any{
			"thread": thread.Id, "subject": fmt.Sprintf("thread %d", i), "imap_uid": i + 1,
			"date": "2026-10-01 09:00:00.000Z", "sender_email": "a@example.org",
		})
	}
	return env
}

func (env *addressFolderEnv) setAddressViews(t *testing.T, on bool) {
	t.Helper()
	env.identity.Set("address_views", on)
	if err := env.app.Save(env.identity); err != nil {
		t.Fatal(err)
	}
}

// dialAddressFolder serves one pre-authenticated imapSession for env.user
// over a loopback listener, so each test drives real IMAP commands. Without
// mailboxes, the session holds the personal inbox only.
func dialAddressFolder(t *testing.T, env *addressFolderEnv, mailboxes ...mailboxContext) *imapclient.Client {
	t.Helper()
	if len(mailboxes) == 0 {
		mailboxes = []mailboxContext{{name: "Owner", mailboxID: env.mailbox.Id}}
	}
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			s := newIMAPSession(env.app)
			s.user = env.user
			s.mailboxIndex = mailboxes
			s.multiMailbox = len(mailboxes) > 1
			return s, &imapserver.GreetingData{PreAuth: true}, nil
		},
		Caps:         mailproto.IMAPCaps,
		InsecureAuth: true,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = srv.Serve(ln)
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client := imapclient.New(conn, nil)
	t.Cleanup(func() {
		client.Close()
		ln.Close()
		srv.Close()
		wg.Wait()
	})
	if err := client.WaitGreeting(); err != nil {
		t.Fatal(err)
	}
	return client
}

func listFolders(t *testing.T, client *imapclient.Client) map[string][]imap.MailboxAttr {
	t.Helper()
	list, err := client.List("", "*", nil).Collect()
	if err != nil {
		t.Fatal(err)
	}
	folders := make(map[string][]imap.MailboxAttr, len(list))
	for _, data := range list {
		folders[data.Mailbox] = data.Attrs
	}
	return folders
}

func TestAddressViewsFor_OrderAndThreshold(t *testing.T) {
	t.Parallel()
	env := seedAddressFolder(t)

	views, err := addressViewsFor(env.app, env.user.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 0 {
		t.Fatalf("switch off: got %v, want no views", views)
	}

	env.setAddressViews(t, true)
	views, err = addressViewsFor(env.app, env.user.Id)
	if err != nil {
		t.Fatal(err)
	}
	want := []addressView{
		{ID: env.billing.Id, Email: addressBill},
		{ID: env.a.Id, Email: addressA},
		{ID: env.b.Id, Email: addressB},
	}
	if !slices.Equal(views, want) {
		t.Fatalf("views = %v, want %v (domain rank, then local part; hidden left out)", views, want)
	}

	env.identity.Set("hidden_address_views", []string{env.hidden.Id, env.billing.Id, env.b.Id})
	if err := env.app.Save(env.identity); err != nil {
		t.Fatal(err)
	}
	views, err = addressViewsFor(env.app, env.user.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 0 {
		t.Fatalf("one visible address: got %v, want no views", views)
	}
}

func TestIMAPAddressFolder_ListsOnlyWhenOn(t *testing.T) {
	env := seedAddressFolder(t)
	client := dialAddressFolder(t, env)

	off := listFolders(t, client)
	for name := range off {
		if name == "Addresses" || isAddressFolder(name) {
			t.Fatalf("switch off: LIST shows %q", name)
		}
	}

	env.setAddressViews(t, true)
	on := listFolders(t, client)
	parent, ok := on["Addresses"]
	if !ok {
		t.Fatal("switch on: LIST does not show Addresses")
	}
	if !slices.Equal(parent, []imap.MailboxAttr{imap.MailboxAttrNoSelect}) {
		t.Errorf("Addresses attrs = %v, want only \\Noselect", parent)
	}
	for _, email := range []string{addressA, addressB, addressBill} {
		if _, ok := on["Addresses/"+email]; !ok {
			t.Errorf("switch on: LIST does not show Addresses/%s", email)
		}
	}
	if _, ok := on["Addresses/"+addressHidden]; ok {
		t.Errorf("LIST shows the hidden address %s", addressHidden)
	}
}

func TestIMAPAddressFolder_ListsUnderPersonalInboxOnly(t *testing.T) {
	env := seedAddressFolder(t)
	env.setAddressViews(t, true)
	shared := saveRealRecord(t, env.app, "mail_mailboxes", map[string]any{
		"address": "support", "domain": env.mailbox.GetString("domain"), "type": "shared",
		"display_name": "Support", "name": "Support",
	})
	client := dialAddressFolder(t, env,
		mailboxContext{name: "Owner", mailboxID: env.mailbox.Id},
		mailboxContext{name: "Support", mailboxID: shared.Id},
	)

	folders := listFolders(t, client)
	if _, ok := folders["Owner/Addresses/"+addressA]; !ok {
		t.Errorf("LIST does not show Owner/Addresses/%s", addressA)
	}
	for name := range folders {
		if name == "Support/Addresses" || isAddressFolder(strings.TrimPrefix(name, "Support/")) {
			t.Errorf("LIST shows %q under the shared inbox", name)
		}
	}
}

func TestIMAPAddressFolder_ListsMoreThanFilterLimit(t *testing.T) {
	env := seedAddressFolder(t)
	env.setAddressViews(t, true)
	client := dialAddressFolder(t, env)

	data, err := client.Select("Addresses/"+addressA, nil).Wait()
	if err != nil {
		t.Fatal(err)
	}
	if data.NumMessages != uint32(env.wantA) {
		t.Fatalf("Addresses/%s holds %d messages, want %d", addressA, data.NumMessages, env.wantA)
	}

	var all imap.SeqSet
	all.AddRange(1, 0)
	fetched, err := client.Fetch(all, &imap.FetchOptions{UID: true}).Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(fetched) != env.wantA {
		t.Fatalf("FETCH returned %d messages, want %d", len(fetched), env.wantA)
	}

	data, err = client.Select("Addresses/"+addressB, nil).Wait()
	if err != nil {
		t.Fatal(err)
	}
	if data.NumMessages != uint32(env.wantB) {
		t.Fatalf("Addresses/%s holds %d messages, want %d", addressB, data.NumMessages, env.wantB)
	}
}

func TestIMAPAddressFolder_StatusCountsMoreThanFilterLimit(t *testing.T) {
	env := seedAddressFolder(t)
	env.setAddressViews(t, true)
	client := dialAddressFolder(t, env)

	data, err := client.Status("Addresses/"+addressA,
		&imap.StatusOptions{NumMessages: true, NumUnseen: true}).Wait()
	if err != nil {
		t.Fatal(err)
	}
	if *data.NumMessages != uint32(env.wantA) {
		t.Fatalf("MESSAGES = %d, want %d", *data.NumMessages, env.wantA)
	}
	if *data.NumUnseen != uint32(env.wantUnseenA) {
		t.Fatalf("UNSEEN = %d, want %d", *data.NumUnseen, env.wantUnseenA)
	}
}

func wantSystemFolderRefusal(t *testing.T, what string, err error) {
	t.Helper()
	var imapErr *imap.Error
	if !errors.As(err, &imapErr) || imapErr.Type != imap.StatusResponseTypeNo ||
		!strings.Contains(imapErr.Text, "system folder") {
		t.Errorf("%s: got %v, want the system folder NO", what, err)
	}
}

func wantReadOnlyRefusal(t *testing.T, what string, err error) {
	t.Helper()
	var imapErr *imap.Error
	if !errors.As(err, &imapErr) || imapErr.Code != imap.ResponseCodeCannot {
		t.Errorf("%s into a read-only folder: got %v, want NO [CANNOT]", what, err)
	}
}

func TestIMAPAddressFolder_ReadOnly(t *testing.T) {
	env := seedAddressFolder(t)
	env.setAddressViews(t, true)
	client := dialAddressFolder(t, env)
	view := "Addresses/" + addressA

	wantSystemFolderRefusal(t, "CREATE Addresses/x", client.Create("Addresses/x", nil).Wait())
	wantSystemFolderRefusal(t, "RENAME of a view", client.Rename(view, "Addresses/renamed", nil).Wait())
	wantSystemFolderRefusal(t, "RENAME into Addresses/", client.Rename("INBOX", "Addresses/x", nil).Wait())
	wantSystemFolderRefusal(t, "DELETE of a view", client.Delete(view).Wait())

	if _, err := client.Select("INBOX", nil).Wait(); err != nil {
		t.Fatal(err)
	}
	_, err := client.Copy(imap.SeqSetNum(1), view).Wait()
	wantReadOnlyRefusal(t, "COPY", err)
	_, err = client.Move(imap.SeqSetNum(1), view).Wait()
	wantReadOnlyRefusal(t, "MOVE", err)
	// The Labels parent gives the same response.
	_, err = client.Copy(imap.SeqSetNum(1), "Labels").Wait()
	wantReadOnlyRefusal(t, "COPY into Labels", err)

	raw := []byte("From: a@example.org\r\nTo: owner@example.org\r\nSubject: appended\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <append-1@example.org>\r\n\r\nbody\r\n")
	appendCmd := client.Append(view, int64(len(raw)), nil)
	if _, err := appendCmd.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := appendCmd.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = appendCmd.Wait()
	wantReadOnlyRefusal(t, "APPEND", err)

	// The refusals must not have touched the thread: it is still in INBOX.
	data, err := client.Select(view, nil).Wait()
	if err != nil {
		t.Fatal(err)
	}
	if data.NumMessages != uint32(env.wantA) {
		t.Fatalf("after the refusals the view holds %d messages, want %d", data.NumMessages, env.wantA)
	}
}

func TestIMAPAddressFolder_OffIsEmpty(t *testing.T) {
	env := seedAddressFolder(t)
	env.setAddressViews(t, true)
	client := dialAddressFolder(t, env)
	view := "Addresses/" + addressA

	selected, err := client.Select(view, nil).Wait()
	if err != nil {
		t.Fatal(err)
	}
	if selected.NumMessages != uint32(env.wantA) {
		t.Fatalf("switch on: the view holds %d messages, want %d", selected.NumMessages, env.wantA)
	}
	env.setAddressViews(t, false)

	// The client still has the view selected.
	search, err := client.UIDSearch(&imap.SearchCriteria{}, nil).Wait()
	if err != nil {
		t.Fatalf("SEARCH in a view after the switch went off: %v", err)
	}
	if uids := search.AllUIDs(); len(uids) != 0 {
		t.Fatalf("SEARCH found %d messages after the switch went off, want 0", len(uids))
	}

	data, err := client.Select(view, nil).Wait()
	if err != nil {
		t.Fatalf("SELECT of a view after the switch went off: %v", err)
	}
	if data.NumMessages != 0 {
		t.Fatalf("view holds %d messages after the switch went off, want 0", data.NumMessages)
	}
}
