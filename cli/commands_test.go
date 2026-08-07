package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"tinycld.org/packages/mail/api"
)

// TestSearchQueryCoversEveryRequestField pins searchQuery (and therefore the
// flag set) against api.SearchRequest: a field added to the contract fails
// here until the CLI maps it.
func TestSearchQueryCoversEveryRequestField(t *testing.T) {
	req := api.SearchRequest{
		Query: "q", MailboxID: "m", Limit: 1, Offset: 2, From: "f", To: "t",
		Subject: "s", HasWords: "w", DateAfter: "2026-01-01", DateBefore: "2026-02-01",
		Folder: "inbox", HasAttachment: true,
	}
	got := searchQuery(req)

	rt := reflect.TypeOf(req)
	for i := 0; i < rt.NumField(); i++ {
		tag := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			t.Fatalf("field %s has no json tag", rt.Field(i).Name)
		}
		if !got.Has(tag) {
			t.Errorf("searchQuery does not map %q — a SearchRequest field the CLI silently drops", tag)
		}
	}
	if len(got) != rt.NumField() {
		t.Errorf("searchQuery emits %d params for %d fields — an extra key the server will ignore", len(got), rt.NumField())
	}
}

func TestSearchSendsFlagsAndRendersResults(t *testing.T) {
	f := mailFixture(t)
	f.searchResponse = api.SearchResponse{
		Items: []api.SearchResultItem{{
			ThreadID: "thr1", Subject: "Quarterly invoice",
			SubjectHighlight: "Quarterly <mark>invoice</mark>",
			SnippetHighlight: "the <mark>invoice</mark> attached",
			LatestDate:       "2026-08-01 10:00:00Z", MessageCount: 2, MailboxID: "mbx1",
		}},
		Total: 1,
	}
	_, c := f.serve()

	out, _, err := runCmd(t, c, "mail", "search", "invoice",
		"--from", "ada@acme.test", "--limit", "5", "--has-attachment", "--folder", "inbox")
	if err != nil {
		t.Fatal(err)
	}
	q := f.lastSearchQuery
	if q.Get("q") != "invoice" || q.Get("from") != "ada@acme.test" ||
		q.Get("limit") != "5" || q.Get("has_attachment") != "true" || q.Get("folder") != "inbox" {
		t.Fatalf("query = %v", q)
	}
	if strings.Contains(out, "<mark>") {
		t.Fatalf("table output must strip <mark>:\n%s", out)
	}

	// --mailbox resolves an address to the id before it hits the wire.
	if _, _, err := runCmd(t, c, "mail", "search", "x", "--mailbox", "team@acme.test"); err != nil {
		t.Fatal(err)
	}
	if f.lastSearchQuery.Get("mailbox_id") != "mbx1" {
		t.Fatalf("mailbox_id = %q, want resolved id", f.lastSearchQuery.Get("mailbox_id"))
	}
}

func TestListBuildsTheAppFilter(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	out, _, err := runCmd(t, c, "mail", "list")
	if err != nil {
		t.Fatal(err)
	}
	want := `mailbox = "mbx1" && mail_thread_state_via_thread.user ?= "user1" && mail_thread_state_via_thread.folder ?= "inbox"`
	if f.lastThreadsFilter != want {
		t.Fatalf("filter:\n got %q\nwant %q", f.lastThreadsFilter, want)
	}
	// Unread + starred flags from the state row.
	if !strings.Contains(out, "●★") {
		t.Fatalf("flags column missing:\n%s", out)
	}
	if !strings.Contains(out, "Ada") {
		t.Fatalf("participants column missing:\n%s", out)
	}

	if _, _, err := runCmd(t, c, "mail", "list", "--folder", "starred"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.lastThreadsFilter, "is_starred ?= true") {
		t.Fatalf("starred filter: %q", f.lastThreadsFilter)
	}
	if _, _, err := runCmd(t, c, "mail", "list", "--folder", "all"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.lastThreadsFilter, "folder ?=") {
		t.Fatalf("folder=all must drop the folder clause: %q", f.lastThreadsFilter)
	}
}

func TestReadMessageConvertsBodyViaFileFetch(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	out, _, err := runCmd(t, c, "mail", "read", "msg1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "From: Ada <ada@acme.test>") {
		t.Fatalf("headers missing:\n%s", out)
	}
	// html2text output, not raw markup.
	if strings.Contains(out, "<p>") || !strings.Contains(out, "Hello") {
		t.Fatalf("body not converted:\n%s", out)
	}

	out, _, err = runCmd(t, c, "mail", "read", "msg1", "--html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<p>Hello <b>there</b>") {
		t.Fatalf("--html must print the raw body:\n%s", out)
	}
}

func TestReadThreadPrintsAllMessagesAndSnippetFallback(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	out, _, err := runCmd(t, c, "mail", "read", "thr1")
	if err != nil {
		t.Fatal(err)
	}
	// Both messages, oldest first; msg2 has no body file so its snippet shows.
	if !strings.Contains(out, "Ada <ada@acme.test>") || !strings.Contains(out, "Bob <bob@acme.test>") {
		t.Fatalf("thread read must include both messages:\n%s", out)
	}
	if strings.Index(out, "Ada") > strings.Index(out, "Bob") {
		t.Fatalf("messages must print oldest-first:\n%s", out)
	}
	if !strings.Contains(out, "thanks") {
		t.Fatalf("empty-body message must fall back to its snippet:\n%s", out)
	}
}

func TestAttachmentsListsCleanNames(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	out, _, err := runCmd(t, c, "mail", "attachments", "msg1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "invoice.pdf") {
		t.Fatalf("stored suffix must be stripped for display:\n%s", out)
	}
	if strings.Contains(out, "invoice_xyzabc1234.pdf") {
		t.Fatalf("stored name leaked into the table:\n%s", out)
	}
}

func TestDownloadWritesCleanName(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()
	dir := t.TempDir()

	if _, _, err := runCmd(t, c, "mail", "download", "msg1", "--out", dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "invoice.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pdf-bytes" {
		t.Fatalf("content = %q", got)
	}

	if _, _, err := runCmd(t, c, "mail", "download", "msg1", "--attachment", "7", "--out", dir); err == nil {
		t.Fatal("out-of-range --attachment must error")
	}

	// --out creates the directory instead of failing on the temp file.
	nested := filepath.Join(dir, "not", "yet", "here")
	if _, _, err := runCmd(t, c, "mail", "download", "msg1", "--out", nested); err != nil {
		t.Fatalf("download must create the --out directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nested, "invoice.pdf")); err != nil {
		t.Fatal(err)
	}
}

func TestSendJSONWhenNoAttachments(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	_, _, err := runCmd(t, c, "mail", "send",
		"--to", "Ada <ada@acme.test>", "--to", "bare@acme.test",
		"--cc", "cc@acme.test", "--subject", "hi", "--body", "line1\nline2")
	if err != nil {
		t.Fatal(err)
	}
	if f.sendJSON == nil {
		t.Fatal("attachment-less send must be plain JSON")
	}
	req := f.sendJSON
	if req.MailboxID != "mbx1" {
		t.Fatalf("mailbox_id = %q, want the default membership", req.MailboxID)
	}
	wantTo := []api.Recipient{{Name: "Ada", Email: "ada@acme.test"}, {Email: "bare@acme.test"}}
	if !reflect.DeepEqual(req.To, wantTo) {
		t.Fatalf("to = %+v", req.To)
	}
	if req.TextBody != "line1\nline2" {
		t.Fatalf("text = %q", req.TextBody)
	}
	if !strings.Contains(req.HTMLBody, "line1<br>line2") {
		t.Fatalf("html = %q", req.HTMLBody)
	}
}

func TestSendMultipartWithAttachments(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()
	attach := filepath.Join(t.TempDir(), "notes.txt")
	os.WriteFile(attach, []byte("attach-bytes"), 0o600)

	_, _, err := runCmd(t, c, "mail", "send",
		"--to", "ada@acme.test", "--subject", "hi", "--body", "b", "--attach", attach)
	if err != nil {
		t.Fatal(err)
	}
	if f.sendMultipart == nil {
		t.Fatal("attachment send must be multipart")
	}
	if f.sendFiles["notes.txt"] != "attach-bytes" {
		t.Fatalf("attachment bytes = %q", f.sendFiles["notes.txt"])
	}
}

func TestSendBodyFromStdin(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	root := newTestRoot(c)
	var out, errBuf bytes.Buffer
	root.SetIn(strings.NewReader("stdin body"))
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"mail", "send", "--to", "a@b.c", "--body-file", "-"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if f.sendJSON == nil || f.sendJSON.TextBody != "stdin body" {
		t.Fatalf("stdin body not read: %+v", f.sendJSON)
	}
}

func TestMailboxesAndStatus(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	out, _, err := runCmd(t, c, "mail", "mailboxes")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"team@acme.test", "Team", "member"} {
		if !strings.Contains(out, want) {
			t.Errorf("mailboxes output missing %q:\n%s", want, out)
		}
	}

	out, _, err = runCmd(t, c, "mail", "status", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []statusRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	if len(rows) != 1 || rows[0].Address != "team@acme.test" {
		t.Fatalf("rows = %+v", rows)
	}
	// The fixture's one state row is unread inbox + starred.
	if rows[0].Inbox != 1 || rows[0].Starred != 1 {
		t.Fatalf("counts = %+v", rows[0])
	}
}
