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
}

func newFakeMail(t *testing.T) *fakeMail {
	return &fakeMail{
		t:         t,
		mailboxes: map[string]*mailbox{}, threads: map[string]*thread{},
		states: map[string]*threadState{}, messages: map[string]*message{},
		bodies: map[string]string{}, attachData: map[string]string{},
		sendFiles: map[string]string{},
	}
}

var (
	reUserEq   = regexp.MustCompile(`^user = "((?:[^"\\]|\\.)*)"$`)
	reThreadEq = regexp.MustCompile(`^thread = "((?:[^"\\]|\\.)*)"$`)
	unquoter   = strings.NewReplacer(`\"`, `"`, `\\`, `\`)
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
		json.NewEncoder(w).Encode(mb)
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
		// The CLI's state filter is user = X && (thread = a || thread = b...);
		// serve every state row for the fixture user — assertions live on the
		// rendered output.
		var out []threadState
		for _, s := range f.states {
			out = append(out, *s)
		}
		listOut(w, out, 1, 1, len(out))
	})

	mux.HandleFunc("GET /api/collections/mail_messages/records", func(w http.ResponseWriter, r *http.Request) {
		m := reThreadEq.FindStringSubmatch(r.URL.Query().Get("filter"))
		if m == nil {
			f.t.Errorf("unsupported messages filter: %q", r.URL.Query().Get("filter"))
		}
		var out []message
		for _, msg := range f.messages {
			if msg.Thread == unquoter.Replace(m[1]) {
				out = append(out, *msg)
			}
		}
		listOut(w, out, 1, 1, len(out))
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
	f.mailboxes["mbx1"] = &mailbox{ID: "mbx1", Address: "team@acme.test", DisplayName: "Team", Type: "shared"}
	f.members = []membership{{ID: "mem1", Mailbox: "mbx1", User: "user1", Role: "member"}}
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
