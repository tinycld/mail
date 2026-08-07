package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/packages/mail/api"
)

// fakeMail is an in-memory stand-in for the server: the mail collections the
// commands read plus the typed /api/mail endpoints. Filters are parsed
// against the exact shapes the CLI builds.
type fakeMail struct {
	t          *testing.T
	mailboxes  map[string]*mailbox
	members    []membership
	threads    map[string]*thread
	states     map[string]*threadState
	messages   map[string]*message
	bodies     map[string]string // message ID -> body_html bytes
	attachData map[string]string // stored name -> bytes

	searchResponse  api.SearchResponse
	lastSearchQuery url.Values

	lastThreadsFilter string

	// send capture
	sendJSON      *api.SendEmailRequest
	sendMultipart *api.SendEmailRequest
	sendFiles     map[string]string

	domains        map[string]*domain
	mailboxDomains map[string]string // mailbox id -> domain id
	aliases        []alias
	labels         []label
	assignments    map[string]*labelAssignment

	statePatches    []map[string]any
	deletedMessages []string
	draftJSON       *api.SaveDraftRequest
	draftMultipart  *api.SaveDraftRequest

	seq int // ids for records the fake creates
}

func newFakeMail(t *testing.T) *fakeMail {
	return &fakeMail{
		t:         t,
		mailboxes: map[string]*mailbox{}, threads: map[string]*thread{},
		states: map[string]*threadState{}, messages: map[string]*message{},
		bodies: map[string]string{}, attachData: map[string]string{},
		sendFiles:   map[string]string{},
		domains:     map[string]*domain{},
		assignments: map[string]*labelAssignment{},
	}
}

var (
	reUserEq       = regexp.MustCompile(`^user = "((?:[^"\\]|\\.)*)"$`)
	reThreadEq     = regexp.MustCompile(`^thread = "((?:[^"\\]|\\.)*)"$`)
	reThreadUser   = regexp.MustCompile(`^thread = "((?:[^"\\]|\\.)*)" && user = "((?:[^"\\]|\\.)*)"$`)
	reMailboxEq    = regexp.MustCompile(`^mailbox = "((?:[^"\\]|\\.)*)"$`)
	reLabelVisible = regexp.MustCompile(`^user = "" \|\| user = "((?:[^"\\]|\\.)*)"$`)
	reAssignment   = regexp.MustCompile(`^label = "((?:[^"\\]|\\.)*)" && record_id = "((?:[^"\\]|\\.)*)" && collection = "((?:[^"\\]|\\.)*)" && user = "((?:[^"\\]|\\.)*)"$`)
	reThreadDraft  = regexp.MustCompile(`^thread = "((?:[^"\\]|\\.)*)" && delivery_status = "draft"$`)
	unquoter       = strings.NewReplacer(`\"`, `"`, `\\`, `\`)
)

func listOut[T any](w http.ResponseWriter, items []T, page, totalPages, totalItems int) {
	if items == nil {
		items = []T{}
	}
	json.NewEncoder(w).Encode(map[string]any{
		"page": page, "perPage": 200, "totalItems": totalItems, "totalPages": totalPages,
		"items": items,
	})
}

func (f *fakeMail) serve() (*httptest.Server, *client.Client) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /oauth/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"sub": "user1"})
	})

	mux.HandleFunc("GET /api/collections/mail_mailbox_members/records", func(w http.ResponseWriter, r *http.Request) {
		m := reUserEq.FindStringSubmatch(r.URL.Query().Get("filter"))
		if m == nil {
			f.t.Errorf("unsupported members filter: %q", r.URL.Query().Get("filter"))
		}
		var out []membership
		for _, mem := range f.members {
			if mem.User == unquoter.Replace(m[1]) {
				out = append(out, mem)
			}
		}
		listOut(w, out, 1, 1, len(out))
	})
	mux.HandleFunc("GET /api/collections/mail_mailboxes/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		mb, ok := f.mailboxes[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"message": "Not found"})
			return
		}
		// The domain relation is on the row too; identity building reads it.
		json.NewEncoder(w).Encode(mailboxWithDomain{
			ID: mb.ID, Address: mb.Address, DisplayName: mb.DisplayName,
			Type: mb.Type, Domain: f.mailboxDomains[mb.ID],
		})
	})

	mux.HandleFunc("GET /api/collections/mail_threads/records", func(w http.ResponseWriter, r *http.Request) {
		f.lastThreadsFilter = r.URL.Query().Get("filter")
		var out []thread
		for _, th := range f.threads {
			out = append(out, *th)
		}
		listOut(w, out, 1, 1, len(out))
	})
	mux.HandleFunc("GET /api/collections/mail_thread_state/records", func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")
		// The single-row lookup the state verbs and labels use.
		if m := reThreadUser.FindStringSubmatch(filter); m != nil {
			var out []threadState
			for _, s := range f.states {
				if s.Thread == unquoter.Replace(m[1]) && s.User == unquoter.Replace(m[2]) {
					out = append(out, *s)
				}
			}
			listOut(w, out, 1, 1, len(out))
			return
		}
		// The list command's filter is user = X && (thread = a || thread = b...);
		// serve every state row for the fixture user — assertions live on the
		// rendered output.
		var out []threadState
		for _, s := range f.states {
			out = append(out, *s)
		}
		listOut(w, out, 1, 1, len(out))
	})
	mux.HandleFunc("PATCH /api/collections/mail_thread_state/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		s, ok := f.states[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		f.statePatches = append(f.statePatches, body)
		if v, ok := body["folder"].(string); ok {
			s.Folder = v
		}
		if v, ok := body["is_read"].(bool); ok {
			s.IsRead = v
		}
		if v, ok := body["is_starred"].(bool); ok {
			s.IsStarred = v
		}
		json.NewEncoder(w).Encode(s)
	})

	mux.HandleFunc("GET /api/collections/mail_messages/records", func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")
		if m := reThreadDraft.FindStringSubmatch(filter); m != nil {
			var out []message
			for _, msg := range f.messages {
				if msg.Thread == unquoter.Replace(m[1]) && msg.DeliveryStatus == "draft" {
					out = append(out, *msg)
				}
			}
			listOut(w, out, 1, 1, len(out))
			return
		}
		m := reThreadEq.FindStringSubmatch(filter)
		if m == nil {
			f.t.Errorf("unsupported messages filter: %q", filter)
		}
		var out []message
		for _, msg := range f.messages {
			if msg.Thread == unquoter.Replace(m[1]) {
				out = append(out, *msg)
			}
		}
		listOut(w, out, 1, 1, len(out))
	})
	mux.HandleFunc("DELETE /api/collections/mail_messages/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := f.messages[r.PathValue("id")]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		delete(f.messages, r.PathValue("id"))
		f.deletedMessages = append(f.deletedMessages, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/collections/mail_threads/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		th, ok := f.threads[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"message": "Not found"})
			return
		}
		json.NewEncoder(w).Encode(th)
	})
	mux.HandleFunc("GET /api/collections/mail_domains/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		d, ok := f.domains[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"message": "Not found"})
			return
		}
		json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("GET /api/collections/mail_mailbox_aliases/records", func(w http.ResponseWriter, r *http.Request) {
		m := reMailboxEq.FindStringSubmatch(r.URL.Query().Get("filter"))
		if m == nil {
			f.t.Errorf("unsupported aliases filter: %q", r.URL.Query().Get("filter"))
		}
		var out []alias
		for _, a := range f.aliases {
			if a.Mailbox == unquoter.Replace(m[1]) {
				out = append(out, a)
			}
		}
		listOut(w, out, 1, 1, len(out))
	})

	mux.HandleFunc("GET /api/collections/labels/records", func(w http.ResponseWriter, r *http.Request) {
		m := reLabelVisible.FindStringSubmatch(r.URL.Query().Get("filter"))
		if m == nil {
			f.t.Errorf("unsupported labels filter: %q", r.URL.Query().Get("filter"))
		}
		var out []label
		for _, l := range f.labels {
			if l.User == "" || l.User == unquoter.Replace(m[1]) {
				out = append(out, l)
			}
		}
		listOut(w, out, 1, 1, len(out))
	})
	mux.HandleFunc("GET /api/collections/label_assignments/records", func(w http.ResponseWriter, r *http.Request) {
		m := reAssignment.FindStringSubmatch(r.URL.Query().Get("filter"))
		if m == nil {
			f.t.Errorf("unsupported assignments filter: %q", r.URL.Query().Get("filter"))
		}
		var out []labelAssignment
		for _, a := range f.assignments {
			if a.Label == unquoter.Replace(m[1]) && a.RecordID == unquoter.Replace(m[2]) &&
				a.Collection == unquoter.Replace(m[3]) && a.User == unquoter.Replace(m[4]) {
				out = append(out, *a)
			}
		}
		listOut(w, out, 1, 1, len(out))
	})
	mux.HandleFunc("POST /api/collections/label_assignments/records", func(w http.ResponseWriter, r *http.Request) {
		var body labelAssignment
		json.NewDecoder(r.Body).Decode(&body)
		f.seq++
		body.ID = fmt.Sprintf("asgn%03d", f.seq)
		f.assignments[body.ID] = &body
		json.NewEncoder(w).Encode(body)
	})
	mux.HandleFunc("DELETE /api/collections/label_assignments/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		delete(f.assignments, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/mail/draft", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				f.t.Errorf("draft multipart parse: %v", err)
			}
			var req api.SaveDraftRequest
			json.Unmarshal([]byte(r.MultipartForm.Value[api.MultipartFieldJSON][0]), &req)
			f.draftMultipart = &req
		} else {
			var req api.SaveDraftRequest
			json.NewDecoder(r.Body).Decode(&req)
			f.draftJSON = &req
		}
		id := "msgDraft"
		if f.draftJSON != nil && f.draftJSON.MessageID != "" {
			id = f.draftJSON.MessageID
		}
		json.NewEncoder(w).Encode(api.SendEmailResponse{MessageID: id, ThreadID: "thrDraft"})
	})
	mux.HandleFunc("GET /api/collections/mail_messages/records/{id}", func(w http.ResponseWriter, r *http.Request) {
		msg, ok := f.messages[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"message": "Not found"})
			return
		}
		json.NewEncoder(w).Encode(msg)
	})

	mux.HandleFunc("GET /api/collections/mail_folder_counts/records", func(w http.ResponseWriter, r *http.Request) {
		listOut(w, f.folderCountRows(), 1, 1, 1)
	})

	mux.HandleFunc("GET /api/files/mail_messages/{id}/{file}", func(w http.ResponseWriter, r *http.Request) {
		msg, ok := f.messages[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if msg.BodyHTML == r.PathValue("file") {
			body := f.bodies[msg.ID]
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.Write([]byte(body))
			return
		}
		if msg.RawHeaders != "" && msg.RawHeaders == r.PathValue("file") {
			data := f.attachData[msg.RawHeaders]
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			w.Write([]byte(data))
			return
		}
		for _, stored := range msg.Attachments {
			if stored == r.PathValue("file") {
				data := f.attachData[stored]
				w.Header().Set("Content-Length", strconv.Itoa(len(data)))
				w.Write([]byte(data))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	})

	mux.HandleFunc("GET /api/mail/search", func(w http.ResponseWriter, r *http.Request) {
		f.lastSearchQuery = r.URL.Query()
		json.NewEncoder(w).Encode(f.searchResponse)
	})

	mux.HandleFunc("POST /api/mail/send", func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "multipart/form-data") {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				f.t.Errorf("send multipart parse: %v", err)
			}
			var req api.SendEmailRequest
			if err := json.Unmarshal([]byte(r.MultipartForm.Value[api.MultipartFieldJSON][0]), &req); err != nil {
				f.t.Errorf("send json field: %v", err)
			}
			f.sendMultipart = &req
			for _, fh := range r.MultipartForm.File[api.MultipartFieldAttachments] {
				src, _ := fh.Open()
				data, _ := io.ReadAll(src)
				src.Close()
				f.sendFiles[fh.Filename] = string(data)
			}
		} else {
			var req api.SendEmailRequest
			json.NewDecoder(r.Body).Decode(&req)
			f.sendJSON = &req
		}
		json.NewEncoder(w).Encode(api.SendEmailResponse{MessageID: "msgNew", ThreadID: "thrNew"})
	})

	srv := httptest.NewServer(mux)
	f.t.Cleanup(srv.Close)
	store := &staticStore{tok: client.TokenSet{
		AccessToken: "test-token", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour),
	}}
	return srv, client.New(srv.URL, store, srv.Client())
}

func (f *fakeMail) folderCountRows() []folderCounts {
	counts := map[string]*folderCounts{}
	for _, s := range f.states {
		th, ok := f.threads[s.Thread]
		if !ok {
			continue
		}
		fc, ok := counts[th.Mailbox]
		if !ok {
			fc = &folderCounts{
				ID: s.User + ":" + th.Mailbox, User: s.User, Mailbox: th.Mailbox,
			}
			counts[th.Mailbox] = fc
		}
		if s.Folder == "inbox" && !s.IsRead {
			fc.Inbox++
		}
		if s.Folder == "drafts" {
			fc.Drafts++
		}
		if s.Folder == "sent" {
			fc.Sent++
		}
		if s.IsStarred {
			fc.Starred++
		}
	}
	var out []folderCounts
	for _, fc := range counts {
		out = append(out, *fc)
	}
	return out
}

// standard fixture: one mailbox, one thread with two messages.
func mailFixture(t *testing.T) *fakeMail {
	f := newFakeMail(t)
	// A mailbox stores the local part; the full address is built with its
	// domain, and aliases the same way.
	f.mailboxes["mbx1"] = &mailbox{ID: "mbx1", Address: "team", DisplayName: "Team", Type: "shared"}
	f.mailboxDomains = map[string]string{"mbx1": "dom1"}
	f.domains["dom1"] = &domain{ID: "dom1", Domain: "acme.test"}
	f.aliases = []alias{{ID: "als1", Mailbox: "mbx1", Address: "billing"}}
	f.members = []membership{{ID: "mem1", Mailbox: "mbx1", User: "user1", Role: "member"}}
	f.labels = []label{
		{ID: "lblWork", Name: "Work", Color: "#0B4F4A", User: ""},
		{ID: "lblMine", Name: "Mine", Color: "#333333", User: "user1"},
		{ID: "lblOther", Name: "Other", Color: "#999999", User: "user2"},
	}
	f.threads["thr1"] = &thread{
		ID: "thr1", Mailbox: "mbx1", Subject: "Quarterly invoice",
		Snippet: "please find attached", MessageCount: 2, LatestDate: "2026-08-01 10:00:00Z",
		Participants: []map[string]string{{"name": "Ada", "email": "ada@acme.test"}},
	}
	f.states["st1"] = &threadState{
		ID: "st1", Thread: "thr1", User: "user1", Folder: "inbox", IsRead: false, IsStarred: true,
	}
	f.messages["msg1"] = &message{
		ID: "msg1", Thread: "thr1", SenderName: "Ada", SenderEmail: "ada@acme.test",
		Date: "2026-08-01 09:00:00Z", Subject: "Quarterly invoice",
		Snippet: "please find attached", BodyHTML: "body_abcdef1234.html",
		Attachments: []string{"invoice_xyzabc1234.pdf"},
		RecipientsTo: []map[string]string{
			{"name": "", "email": "team@acme.test"},
		},
	}
	f.bodies["msg1"] = "<p>Hello <b>there</b><br>regards</p>"
	f.attachData["invoice_xyzabc1234.pdf"] = "pdf-bytes"
	f.messages["msg2"] = &message{
		ID: "msg2", Thread: "thr1", SenderName: "Bob", SenderEmail: "bob@acme.test",
		Date: "2026-08-01 10:00:00Z", Subject: "Re: Quarterly invoice",
		Snippet: "thanks", BodyHTML: "",
	}
	return f
}

type staticStore struct{ tok client.TokenSet }

func (s *staticStore) Load() (client.TokenSet, error) { return s.tok, nil }
func (s *staticStore) Save(t client.TokenSet) error   { s.tok = t; return nil }

func newTestRoot(c *client.Client) *cobra.Command {
	root := &cobra.Command{Use: "tinycld", SilenceUsage: true, SilenceErrors: true}
	pf := root.PersistentFlags()
	pf.String("output", "table", "")
	pf.Bool("json", false, "")
	pf.String("context", "", "")
	pf.Bool("quiet", false, "")
	pf.Bool("no-color", false, "")
	pf.Bool("yes", false, "")
	Register(root, c)
	return root
}

func runCmd(t *testing.T, c *client.Client, args ...string) (string, string, error) {
	t.Helper()
	root := newTestRoot(c)
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errBuf.String(), err
}

var _ = fmt.Sprint
