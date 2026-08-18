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
		Folder: "inbox", HasAttachment: true, Exclude: "spam",
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
	// Addresses are FULL addresses: the stored value is only a local part, so
	// printing it bare would show something you cannot send to or pass to
	// --from. Aliases are listed too, since they are also sendable.
	for _, want := range []string{"team@acme.test", "billing@acme.test", "Team", "member", "alias"} {
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

func TestStateVerbsPatchOneField(t *testing.T) {
	cases := []struct {
		args  []string
		field string
		want  any
	}{
		{[]string{"archive", "thr1"}, "folder", "archive"},
		{[]string{"trash", "thr1"}, "folder", "trash"},
		{[]string{"spam", "thr1"}, "folder", "spam"},
		{[]string{"star", "thr1"}, "is_starred", true},
		{[]string{"unstar", "thr1"}, "is_starred", false},
		{[]string{"move", "inbox", "thr1"}, "folder", "inbox"},
		{[]string{"mark", "read", "thr1"}, "is_read", true},
		{[]string{"mark", "unread", "thr1"}, "is_read", false},
	}
	for _, tc := range cases {
		f := mailFixture(t)
		_, c := f.serve()
		if _, _, err := runCmd(t, c, append([]string{"mail"}, tc.args...)...); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if len(f.statePatches) != 1 {
			t.Fatalf("%v: patches = %v", tc.args, f.statePatches)
		}
		patch := f.statePatches[0]
		// Exactly one field: the app's actions never touch anything else.
		if len(patch) != 1 {
			t.Errorf("%v: patch must set one field, got %v", tc.args, patch)
		}
		if patch[tc.field] != tc.want {
			t.Errorf("%v: patch[%s] = %v, want %v", tc.args, tc.field, patch[tc.field], tc.want)
		}
	}
}

func TestStateVerbsRejectBadInput(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	if _, _, err := runCmd(t, c, "mail", "move", "nowhere", "thr1"); err == nil {
		t.Fatal("an unknown folder must be refused")
	}
	if _, _, err := runCmd(t, c, "mail", "mark", "sideways", "thr1"); err == nil {
		t.Fatal("mark takes read or unread only")
	}
	// A thread with no state row for the caller is not their mail.
	if _, _, err := runCmd(t, c, "mail", "archive", "thrNotMine"); err == nil {
		t.Fatal("a thread with no state row must error")
	}
	if len(f.statePatches) != 0 {
		t.Fatalf("no patch should have been sent: %v", f.statePatches)
	}
}

func TestStateVerbsAcceptSeveralThreads(t *testing.T) {
	f := mailFixture(t)
	f.threads["thr2"] = &thread{ID: "thr2", Mailbox: "mbx1", Subject: "Second"}
	f.states["st2"] = &threadState{ID: "st2", Thread: "thr2", User: "user1", Folder: "inbox"}
	_, c := f.serve()

	if _, _, err := runCmd(t, c, "mail", "archive", "thr1", "thr2"); err != nil {
		t.Fatal(err)
	}
	if len(f.statePatches) != 2 {
		t.Fatalf("patches = %v", f.statePatches)
	}
	if f.states["st1"].Folder != "archive" || f.states["st2"].Folder != "archive" {
		t.Fatalf("both threads must be archived: %+v %+v", f.states["st1"], f.states["st2"])
	}
}

func TestLabelsListShowsSharedAndOwn(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	out, _, err := runCmd(t, c, "mail", "labels")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Work") || !strings.Contains(out, "Mine") {
		t.Fatalf("shared and own labels must both list:\n%s", out)
	}
	if strings.Contains(out, "Other") {
		t.Fatalf("another user's private label must not list:\n%s", out)
	}
}

func TestLabelAddIsIdempotentAndRemoveWorks(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	if _, _, err := runCmd(t, c, "mail", "label", "add", "thr1", "Work"); err != nil {
		t.Fatal(err)
	}
	if len(f.assignments) != 1 {
		t.Fatalf("assignments = %v", f.assignments)
	}
	for _, a := range f.assignments {
		// The assignment points at the caller's STATE row, not the thread.
		if a.RecordID != "st1" || a.Collection != "mail_thread_state" || a.Label != "lblWork" || a.User != "user1" {
			t.Fatalf("assignment = %+v", a)
		}
	}

	// The web insert has no dedup guard; the CLI must not create a second row.
	if _, _, err := runCmd(t, c, "mail", "label", "add", "thr1", "Work"); err != nil {
		t.Fatal(err)
	}
	if len(f.assignments) != 1 {
		t.Fatalf("a repeated add must not duplicate: %v", f.assignments)
	}

	if _, _, err := runCmd(t, c, "mail", "label", "remove", "thr1", "Work"); err != nil {
		t.Fatal(err)
	}
	if len(f.assignments) != 0 {
		t.Fatalf("remove must delete the assignment: %v", f.assignments)
	}
	// Removing an absent label is a no-op, not an error.
	if _, _, err := runCmd(t, c, "mail", "label", "remove", "thr1", "Work"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCmd(t, c, "mail", "label", "add", "thr1", "Nonexistent"); err == nil {
		t.Fatal("an unknown label must error")
	}
}

func TestSendFromResolvesAliasAddress(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	// An alias stores only its local part; the full address is built with the
	// mailbox's domain.
	_, _, err := runCmd(t, c, "mail", "send", "--to", "a@b.c",
		"--subject", "s", "--body", "b", "--from", "billing@acme.test")
	if err != nil {
		t.Fatal(err)
	}
	if f.sendJSON.MailboxID != "mbx1" || f.sendJSON.AliasID != "als1" {
		t.Fatalf("send request = %+v", f.sendJSON)
	}

	f.sendJSON = nil
	_, _, err = runCmd(t, c, "mail", "send", "--to", "a@b.c",
		"--subject", "s", "--body", "b", "--from", "team@acme.test")
	if err != nil {
		t.Fatal(err)
	}
	if f.sendJSON.AliasID != "" {
		t.Fatalf("the mailbox's own address must carry no alias: %+v", f.sendJSON)
	}

	if _, _, err := runCmd(t, c, "mail", "send", "--to", "a@b.c", "--body", "b",
		"--from", "stranger@example.com"); err == nil {
		t.Fatal("an address that is not yours must be refused")
	}
	if _, _, err := runCmd(t, c, "mail", "send", "--to", "a@b.c", "--body", "b",
		"--from", "team@acme.test", "--mailbox", "mbx1"); err == nil {
		t.Fatal("--from with --mailbox must be refused")
	}
}

func TestReplyRecipientsAndSubject(t *testing.T) {
	f := mailFixture(t)
	// A message addressed to the mailbox with another person in Cc.
	f.messages["msg1"].RecipientsCc = []map[string]string{
		{"name": "Cara", "email": "cara@acme.test"},
	}
	_, c := f.serve()

	if _, _, err := runCmd(t, c, "mail", "reply", "msg1", "--body", "thanks"); err != nil {
		t.Fatal(err)
	}
	req := f.sendJSON
	if req.Subject != "Re: Quarterly invoice" {
		t.Fatalf("subject = %q", req.Subject)
	}
	// Plain reply: the sender alone.
	if len(req.To) != 1 || req.To[0].Email != "ada@acme.test" {
		t.Fatalf("reply recipients = %+v", req.To)
	}
	// in_reply_to carries the record id; the server derives the RFC header.
	if req.InReplyToMessageID != "msg1" {
		t.Fatalf("in_reply_to = %q", req.InReplyToMessageID)
	}

	f.sendJSON = nil
	if _, _, err := runCmd(t, c, "mail", "reply", "msg1", "--all", "--body", "thanks"); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range f.sendJSON.To {
		got[r.Email] = true
	}
	// Sender + To + Cc, minus our own address, all in To (the app does the same).
	if !got["ada@acme.test"] || !got["cara@acme.test"] {
		t.Fatalf("reply-all recipients = %+v", f.sendJSON.To)
	}
	if got["team@acme.test"] {
		t.Fatalf("reply-all must drop our own address: %+v", f.sendJSON.To)
	}
	if len(f.sendJSON.Cc) != 0 {
		t.Fatalf("reply-all puts everyone in To, not Cc: %+v", f.sendJSON.Cc)
	}

	// An already-prefixed subject is not double-prefixed.
	f.messages["msg1"].Subject = "Re: Quarterly invoice"
	f.sendJSON = nil
	if _, _, err := runCmd(t, c, "mail", "reply", "msg1", "--body", "x"); err != nil {
		t.Fatal(err)
	}
	if f.sendJSON.Subject != "Re: Quarterly invoice" {
		t.Fatalf("subject = %q", f.sendJSON.Subject)
	}
}

func TestReplyToThreadAnswersLatestMessage(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	if _, _, err := runCmd(t, c, "mail", "reply", "thr1", "--body", "ok"); err != nil {
		t.Fatal(err)
	}
	// msg2 is the newest in the fixture thread.
	if f.sendJSON.InReplyToMessageID != "msg2" {
		t.Fatalf("in_reply_to = %q, want the thread's latest message", f.sendJSON.InReplyToMessageID)
	}
	if f.sendJSON.To[0].Email != "bob@acme.test" {
		t.Fatalf("recipients = %+v", f.sendJSON.To)
	}
}

func TestDraftCreateAndUpdate(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	if _, _, err := runCmd(t, c, "mail", "draft", "--to", "a@b.c",
		"--subject", "later", "--body", "wip"); err != nil {
		t.Fatal(err)
	}
	if f.draftJSON == nil || f.draftJSON.Subject != "later" || f.draftJSON.MessageID != "" {
		t.Fatalf("draft request = %+v", f.draftJSON)
	}

	f.draftJSON = nil
	if _, _, err := runCmd(t, c, "mail", "draft", "--message-id", "msgDraft",
		"--subject", "revised", "--body", "more"); err != nil {
		t.Fatal(err)
	}
	if f.draftJSON.MessageID != "msgDraft" || f.draftJSON.Subject != "revised" {
		t.Fatalf("draft update = %+v", f.draftJSON)
	}
}

// The save endpoint replaces the whole draft rather than patching it, so an
// update that names only one field must resend the rest — otherwise
// `--subject x` silently drops the recipients and body already on the draft.
func TestDraftUpdateKeepsUnspecifiedFields(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()
	if _, _, err := runCmd(t, c, "mail", "draft",
		"--message-id", "msgDraft", "--subject", "revised"); err != nil {
		t.Fatal(err)
	}
	if f.draftJSON.Subject != "revised" {
		t.Fatalf("subject = %q, want the new one", f.draftJSON.Subject)
	}
	if len(f.draftJSON.To) != 1 || f.draftJSON.To[0].Email != "a@b.c" {
		t.Fatalf("unspecified --to must be preserved, got %+v", f.draftJSON.To)
	}
	if len(f.draftJSON.Cc) != 1 || f.draftJSON.Cc[0].Email != "cee@acme.test" {
		t.Fatalf("unspecified --cc must be preserved, got %+v", f.draftJSON.Cc)
	}
	// An empty TextBody is how the endpoint is told to keep the stored body.
	if f.draftJSON.TextBody != "" {
		t.Fatalf("an untouched body must not be resent, got %q", f.draftJSON.TextBody)
	}
	// The recipients are replaceable — Changed(), not emptiness, is the test.
	f.draftJSON = nil
	if _, _, err := runCmd(t, c, "mail", "draft",
		"--message-id", "msgDraft", "--to", "new@acme.test"); err != nil {
		t.Fatal(err)
	}
	if len(f.draftJSON.To) != 1 || f.draftJSON.To[0].Email != "new@acme.test" {
		t.Fatalf("an explicit --to must win, got %+v", f.draftJSON.To)
	}
	if f.draftJSON.Subject != "later" {
		t.Fatalf("unspecified --subject must be preserved, got %q", f.draftJSON.Subject)
	}
}

// Clearing a body isn't expressible through the save endpoint (an empty
// TextBody means "keep what's stored"), so it must be refused rather than
// reported as an update that did nothing.
func TestDraftUpdateRefusesToClearBody(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	if _, _, err := runCmd(t, c, "mail", "draft",
		"--message-id", "msgDraft", "--body", ""); err == nil {
		t.Fatal("clearing a body must be refused, not silently ignored")
	}
}

func TestDraftSendSendsThenDeletes(t *testing.T) {
	f := mailFixture(t)
	f.threads["thrD"] = &thread{ID: "thrD", Mailbox: "mbx1", Subject: "Ready to go"}
	f.states["stD"] = &threadState{ID: "stD", Thread: "thrD", User: "user1", Folder: "drafts"}
	f.messages["msgD"] = &message{
		ID: "msgD", Thread: "thrD", Subject: "Ready to go", DeliveryStatus: "draft",
		BodyHTML: "bodyD_abcdef1234.html", Alias: "als1",
		RecipientsTo:  []map[string]string{{"name": "Ada", "email": "ada@acme.test"}},
		RecipientsCc:  []map[string]string{},
		RecipientsBcc: []map[string]string{},
		Attachments:   []string{"draft-notes_xyzabc1234.txt"},
	}
	f.bodies["msgD"] = "<p>the body</p>"
	f.attachData["draft-notes_xyzabc1234.txt"] = "draft attachment bytes"
	_, c := f.serve()

	if _, _, err := runCmd(t, c, "mail", "draft", "send", "msgD"); err != nil {
		t.Fatal(err)
	}
	// Attachments cannot be referenced by id on send, so they round-trip.
	if f.sendMultipart == nil {
		t.Fatal("a draft with attachments must be sent as multipart")
	}
	req := f.sendMultipart
	if req.MailboxID != "mbx1" || req.AliasID != "als1" {
		t.Fatalf("send request = %+v", req)
	}
	if len(req.To) != 1 || req.To[0].Email != "ada@acme.test" {
		t.Fatalf("recipients = %+v", req.To)
	}
	if !strings.Contains(req.TextBody, "the body") {
		t.Fatalf("body = %q", req.TextBody)
	}
	if f.sendFiles["draft-notes.txt"] != "draft attachment bytes" {
		t.Fatalf("attachment bytes = %q", f.sendFiles["draft-notes.txt"])
	}
	// The draft is removed, or the thread would show both copies.
	if len(f.deletedMessages) != 1 || f.deletedMessages[0] != "msgD" {
		t.Fatalf("deleted = %v", f.deletedMessages)
	}

	// A sent message is not a draft, and the endpoint does not check.
	if _, _, err := runCmd(t, c, "mail", "draft", "send", "msg1"); err == nil {
		t.Fatal("draft send must refuse a non-draft message")
	}
}

func TestReadMarksThreadReadUnlessNoMark(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	if _, _, err := runCmd(t, c, "mail", "read", "msg1"); err != nil {
		t.Fatal(err)
	}
	if !f.states["st1"].IsRead {
		t.Fatal("read must mark the thread read, as opening it in the app does")
	}

	f.states["st1"].IsRead = false
	f.statePatches = nil
	if _, _, err := runCmd(t, c, "mail", "read", "msg1", "--no-mark"); err != nil {
		t.Fatal(err)
	}
	if f.states["st1"].IsRead || len(f.statePatches) != 0 {
		t.Fatalf("--no-mark must leave unread state alone (patches: %v)", f.statePatches)
	}

	// Already-read threads need no write at all.
	f.states["st1"].IsRead = true
	f.statePatches = nil
	if _, _, err := runCmd(t, c, "mail", "read", "msg1"); err != nil {
		t.Fatal(err)
	}
	if len(f.statePatches) != 0 {
		t.Fatalf("an already-read thread must not be patched: %v", f.statePatches)
	}
}

func TestReadRawHeaders(t *testing.T) {
	f := mailFixture(t)
	f.messages["msg1"].RawHeaders = "headers_abcdef1234.txt"
	f.attachData["headers_abcdef1234.txt"] = "Subject: Quarterly invoice\r\nFrom: ada@acme.test\r\n"
	_, c := f.serve()

	out, _, err := runCmd(t, c, "mail", "read", "msg1", "--raw")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "From: ada@acme.test") {
		t.Fatalf("raw headers not printed:\n%s", out)
	}

	// Only IMAP-appended messages carry them; say so rather than print nothing.
	f.messages["msg1"].RawHeaders = ""
	out, stderr, err := runCmd(t, c, "mail", "read", "msg1", "--raw")
	if err != nil {
		t.Fatal(err)
	}
	if out != "" {
		t.Fatalf("stdout = %q, want empty", out)
	}
	if !strings.Contains(stderr, "no stored raw headers") {
		t.Fatalf("stderr = %q", stderr)
	}
}

// `mail read --json` emitted the record verbatim, whose `body_html` is only the
// stored FILE NAME — so a JSON consumer saw every header and no content, with
// no way to fetch it. The JSON now carries the body the table output prints.
func TestReadJSONIncludesTheBody(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	out, _, err := runCmd(t, c, "mail", "read", "msg1", "--output", "json")
	if err != nil {
		t.Fatal(err)
	}
	var msgs []map[string]any
	if err := json.Unmarshal([]byte(out), &msgs); err != nil {
		t.Fatalf("--json must emit a JSON array: %v\n%s", err, out)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d", len(msgs))
	}

	body, _ := msgs[0]["body"].(string)
	if !strings.Contains(body, "Hello") {
		t.Errorf("body must carry the converted text, got %q", body)
	}
	if strings.Contains(body, "<p>") {
		t.Errorf("body must be the text rendering, not raw markup: %q", body)
	}
	html, _ := msgs[0]["body_html"].(string)
	if !strings.Contains(html, "<p>Hello") {
		t.Errorf("body_html must carry the raw markup, got %q", html)
	}
	// The stored filename is an internal detail and must not be what a caller
	// reads out of body_html.
	if strings.Contains(html, "body_abcdef1234.html") {
		t.Error("body_html leaked the stored file name instead of the content")
	}
}

// A message with no stored body falls back to its snippet, matching the table
// path — an empty string would read as "this message has no content".
func TestReadJSONFallsBackToSnippet(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	out, _, err := runCmd(t, c, "mail", "read", "msg2", "--output", "json")
	if err != nil {
		t.Fatal(err)
	}
	var msgs []map[string]any
	if err := json.Unmarshal([]byte(out), &msgs); err != nil {
		t.Fatal(err)
	}
	if body, _ := msgs[0]["body"].(string); body != "thanks" {
		t.Errorf("body = %q, want the snippet fallback", body)
	}
}

// `reply --all --from <addr>` mailed the caller. The --from branch built a
// partial identity carrying only the mailbox/alias ids, so the self-dedup —
// which compares against identity.Address — had an empty string to match on.
func TestReplyAllWithExplicitFromDropsOurOwnAddress(t *testing.T) {
	f := mailFixture(t)
	// The mailbox address is in To, so a correct reply-all must drop it.
	f.messages["msg1"].RecipientsCc = []map[string]string{
		{"name": "Cara", "email": "cara@acme.test"},
	}
	_, c := f.serve()

	if _, _, err := runCmd(t, c, "mail", "reply", "msg1", "--all",
		"--from", "team@acme.test", "--body", "thanks"); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range f.sendJSON.To {
		got[r.Email] = true
	}
	if got["team@acme.test"] {
		t.Errorf("reply-all --from must still drop our own address: %+v", f.sendJSON.To)
	}
	// The real recipients are untouched.
	if !got["ada@acme.test"] || !got["cara@acme.test"] {
		t.Errorf("reply-all --from dropped a real recipient: %+v", f.sendJSON.To)
	}
	// And the explicit --from is still honored as the sending identity.
	if f.sendJSON.MailboxID != "mbx1" {
		t.Errorf("mailbox_id = %q, want the --from mailbox", f.sendJSON.MailboxID)
	}
}

// An alias --from drops the ALIAS address, not the mailbox's primary.
func TestReplyAllWithAliasFromDropsTheAliasAddress(t *testing.T) {
	f := mailFixture(t)
	f.messages["msg1"].RecipientsTo = []map[string]string{
		{"name": "", "email": "billing@acme.test"},
	}
	_, c := f.serve()

	if _, _, err := runCmd(t, c, "mail", "reply", "msg1", "--all",
		"--from", "billing@acme.test", "--body", "thanks"); err != nil {
		t.Fatal(err)
	}
	for _, r := range f.sendJSON.To {
		if r.Email == "billing@acme.test" {
			t.Fatalf("reply-all must drop the alias we are sending as: %+v", f.sendJSON.To)
		}
	}
	if f.sendJSON.AliasID != "als1" {
		t.Errorf("alias_id = %q, want the --from alias", f.sendJSON.AliasID)
	}
}

// --mailbox says "id or address" but matched only PRIMARY addresses, so an
// alias — an address the user sees on every other surface — was rejected.
func TestMailboxFlagAcceptsAliasAddresses(t *testing.T) {
	f := mailFixture(t)
	_, c := f.serve()

	for _, ref := range []string{"mbx1", "team@acme.test", "billing@acme.test"} {
		if _, _, err := runCmd(t, c, "mail", "list", "--mailbox", ref); err != nil {
			t.Errorf("--mailbox %q must resolve: %v", ref, err)
			continue
		}
		if !strings.Contains(f.lastThreadsFilter, `mailbox = "mbx1"`) {
			t.Errorf("--mailbox %q resolved to the wrong mailbox: %q", ref, f.lastThreadsFilter)
		}
	}

	// An address that is nobody's still fails, with a message naming the flag.
	if _, _, err := runCmd(t, c, "mail", "list", "--mailbox", "nope@acme.test"); err == nil {
		t.Error("an unknown mailbox address must be refused")
	}
}
