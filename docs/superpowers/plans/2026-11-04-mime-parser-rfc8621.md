# MIME parsing by RFC 8621 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `parseRFC5322`'s hand-written part loop with a salvaging MIME reader plus the RFC 8621 §4.1.4 `parseStructure` algorithm, gated by a vendored corpus of about 334 messages, and send subject-only and file-only messages.

**Architecture:** `parseRFC5322` becomes three steps in package `mail` (`server/`):
1. `readTree` (`mimetree.go`) salvages a part tree from any input. It has its own lenient header parser, boundary splitter and decoders, and uses go-message only for the `textproto.Header` type and `go-message/charset`.
2. `parseStructure` (`bodystructure.go`) is a direct port of the RFC algorithm.
3. `flatten` (`bodystructure.go`) maps the RFC's lists onto `storedMessage`.

A corpus under `server/testdata/corpus/` (fetched at pinned commits by a Go tool, with stalwart's expected parses converted to a compact form) and four corpus tests gate the result.

**Tech Stack:** Go; `github.com/emersion/go-message` v0.18.2 (`textproto`, `charset`, `mail`); standard library `mime`, `encoding/base64`.

**Spec:** `mail/docs/superpowers/specs/2026-11-04-mime-parser-rfc8621-design.md`

## Global Constraints

- **Repo and branch:** `mail` (`~/code/tinycld/mail`), branch `mime-parser-rfc8621` (already created; the spec commits are on it).
- **Go tests:** run from `mail/server` with the app shell's workspace, because the mail module has no replaces of its own:
  `cd ~/code/tinycld/mail/server && GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test ./...`
  The whole package check, the one CI runs: `cd ~/code/tinycld/mail && pnpm exec tinycld-pkg check`.
- **Accept liberally:** nothing in the parse path returns an error or drops bytes that it could keep. What cannot be decoded is kept undecoded; what cannot be classified is kept as a file.
- **Dependencies:** no new modules. `go-message/charset` is in go-message v0.18.2, which the module already requires. Do not import `golang.org/x/text` directly: use `charset.Reader("windows-1252", …)`.
- **No content in logs:** never log names, addresses or body text.
- **Out of scope:** `buildRFC5322`, IMAP FETCH, the storage schema, and the Postmark inbound webhook (`postmark.go` `ParseInbound`) do not change.
- **Postmark:** refuses `TextBody: ""` with no `HtmlBody` (ErrorCode 300) and accepts `TextBody: " "`. This was checked on 2026-11-04 with the test token.
- **Style:** comments explain why; four-space-free Go (gofmt); `var xLog = logging.ForPackage("mail")` for logging; no `console`/`fmt.Print` in runtime code.
- **Commits:** conventional commits, no mention of Claude.

## Review Focus

These are the inputs most likely to hurt a real user that no corpus message is sure to cover. Each one has a test in the task named:

1. **Mixed CRLF and LF line endings in one message:** parts still split at every delimiter. (Task 2, `TestSplitMultipart_MixedLineEndings`)
2. **A boundary string that appears inside a line of content:** not taken as a delimiter. (Task 2, `TestSplitMultipart_BoundaryInsideALineIsContent`)
3. **An RFC 2047 encoded filename in a charset nobody knows:** the part is kept, its name kept as written, and a warning is added. (Task 2, `TestReadTree_UndecodableFilenameIsKept`)
4. **base64 on one 100 KB line with no line breaks:** decoded in full. (Task 2, `TestDecodeBase64Lenient_OneLongLine`)
5. **A `message/rfc822` part whose inner message is broken:** the outer message parses, and the inner one is kept as an `.eml` file. (Task 4, `TestParseRFC5322_BrokenInnerMessageIsAFile`)

---

## File Structure

`mail/server/`:
- `smtp_session.go`: modify. `submissionBodies` (Task 1); the parse call and the log (Tasks 4 and 8).
- `mimetree.go`: create. `mimePart`, `parseWarning`, `readTree`, the header parser, the splitter and the decoders (Task 2).
- `bodystructure.go`: create. `bodyLists`, `parseStructure`, `flatten` (Task 3).
- `imap_rfc5322.go`: modify. `parseRFC5322` uses the three steps and the old part loop is deleted (Task 4).
- `imap_session.go`, `smtp_provider.go`: modify. The new `parseRFC5322` signature (Task 4); logging (Task 8).
- `parsereport.go`: create. `parseReport`, the structure summary, and the log (Tasks 4 and 8).
- Tests:
  - `smtp_submission_bodies_test.go` (Task 1)
  - `mimetree_test.go` (Task 2)
  - `bodystructure_test.go` (Task 3)
  - `imap_rfc5322_test.go`, modified (Task 4)
  - `corpus_test.go` (Task 6)
  - `corpus_fuzz_test.go` (Task 7)
  - `parsereport_test.go` (Task 8)
- `testdata/corpus/`:
  - `tools/fetch/main.go` (Task 5)
  - `tools/anonymize/main.go` and `tools/anonymize/main_test.go` (Task 5)
  - the vendored sources under `stalwart/`, `enmime/`, `mimekit/`, `zbateson/`, `cpython/` and `thunderbird/` (Task 5)
  - `faults/` (Task 5)
  - `known_deviations.txt` (Task 6)
  - `clients/.keep` (Task 5)

`mail/`:
- `README.md`: modify, the server layout lines (Task 9).
- The spec: modify, the rulings below (Task 9).

---

### Task 1: Send a message with no body

**Files:**
- Modify: `mail/server/smtp_session.go` (the `SendRequest` literal at about line 340)
- Test: `mail/server/smtp_submission_bodies_test.go`

**Interfaces:**
- Produces: `func submissionBodies(msg *storedMessage) (text, html string)`

- [ ] **Step 1: Write the failing test**

```go
package mail

import "testing"

// A message with only a subject, or a subject and files, is valid mail, but
// Postmark refuses a send with no body at all (ErrorCode 300). It accepts
// white space, so such a message is sent with a single space.
func TestSubmissionBodies(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name               string
		text, html         string
		wantText, wantHTML string
	}{
		{"no body", "", "", " ", ""},
		{"white space only", "\r\n", "", " ", ""},
		{"text", "hello", "", "hello", ""},
		{"html only", "", "<p>hi</p>", "", "<p>hi</p>"},
		{"both", "hi", "<p>hi</p>", "hi", "<p>hi</p>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, html := submissionBodies(&storedMessage{TextBody: tc.text, HTMLBody: tc.html})
			if text != tc.wantText || html != tc.wantHTML {
				t.Fatalf("bodies = %q, %q; want %q, %q", text, html, tc.wantText, tc.wantHTML)
			}
		})
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd ~/code/tinycld/mail/server && GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test -run TestSubmissionBodies .`
Expected: FAIL to compile, `undefined: submissionBodies`.

- [ ] **Step 3: Implement**

In `smtp_session.go`, add above the function that builds `sendReq`:

```go
// submissionBodies is what SMTP submission hands the provider. A message
// with only a subject, or a subject and files, is valid mail, but Postmark
// refuses a send with no body at all (ErrorCode 300) and accepts white
// space, so it goes out as a single space. The stored copy keeps its empty
// bodies.
func submissionBodies(msg *storedMessage) (text, html string) {
	if strings.TrimSpace(msg.TextBody) == "" && strings.TrimSpace(msg.HTMLBody) == "" {
		return " ", ""
	}
	return msg.TextBody, msg.HTMLBody
}
```

and in the `SendRequest` literal replace

```go
		HTMLBody:    msg.HTMLBody,
		TextBody:    msg.TextBody,
```

with

```go
		HTMLBody:    html,
		TextBody:    text,
```

declaring `text, html := submissionBodies(msg)` on the line before `sendReq := &SendRequest{`.

- [ ] **Step 4: Run the tests**

Run: `GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test -run 'TestSubmissionBodies|SMTP|Smtp' .`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd ~/code/tinycld/mail
git add server/smtp_session.go server/smtp_submission_bodies_test.go
git commit -m "fix(mail): send a message with no body as a single space

A subject-only or files-only message is valid mail, but Postmark refuses
a send with no body (ErrorCode 300), so the client got a 451 for ever."
```

This task stands alone; it can ship as a patch release before the rest.

---

### Task 2: `readTree`, the salvaging reader

**Files:**
- Create: `mail/server/mimetree.go`
- Test: `mail/server/mimetree_test.go`

**Interfaces:**
- Produces (used by Tasks 3, 4, 6, 7, 8):

```go
type mimePart struct {
	Index       int               // depth-first position, 0 at the root
	Header      textproto.Header  // the part's own fields (go-message textproto)
	Type        string            // "type/subtype", lower case
	Params      map[string]string // Content-Type parameters, lower-case names
	Disposition string            // "inline", "attachment" or ""
	Name        string            // filename or Content-Type name, decoded where possible
	ContentID   string            // as written, angle brackets included
	Body        []byte            // decoded content; nil for a multipart
	RawSize     int               // non-blank bytes of the part's body as it was in the message
	Children    []*mimePart       // multipart/* only
}
func (p *mimePart) isMultipart() bool
type parseWarning struct{ Part int; Kind, Detail string }
func readTree(raw []byte) (*mimePart, []parseWarning)
func leaves(root *mimePart) []*mimePart // depth-first, non-multipart parts
```

Warning kinds: `bad-header`, `bad-content-type`, `bad-header-param`, `bad-encoding`, `unknown-charset`, `invalid-utf8`, `bad-multipart`, `truncated`, `too-deep`.

- [ ] **Step 1: Write the failing tests**

Create `mail/server/mimetree_test.go`:

```go
package mail

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func crlf(s string) []byte { return []byte(strings.ReplaceAll(s, "\n", "\r\n")) }

func hasWarning(ws []parseWarning, kind string) bool {
	for _, w := range ws {
		if w.Kind == kind {
			return true
		}
	}
	return false
}

func TestParseHeaderBlock(t *testing.T) {
	t.Parallel()

	h, body, problem := parseHeaderBlock(crlf("Subject: one\n two\nFrom: a@example.org\n\nbody\n"))
	if problem != "" || h.Get("Subject") != "one two" || h.Get("From") != "a@example.org" || string(body) != "body\r\n" {
		t.Fatalf("got subject %q from %q body %q problem %q", h.Get("Subject"), h.Get("From"), body, problem)
	}

	// A broken line between fields is skipped; the fields after it stay.
	h, body, problem = parseHeaderBlock(crlf("Subject: s\nthis is not a field\nContent-Type: text/html\n\n<p>x</p>"))
	if h.Get("Content-Type") != "text/html" || string(body) != "<p>x</p>" || problem == "" {
		t.Fatalf("skip: ct %q body %q problem %q", h.Get("Content-Type"), body, problem)
	}

	// A broken line with no field after it ends the header; nothing is lost.
	h, body, problem = parseHeaderBlock(crlf("Subject: s\nnot a field\nmore text\n"))
	if h.Get("Subject") != "s" || string(body) != "not a field\r\nmore text\r\n" || problem == "" {
		t.Fatalf("end: body %q problem %q", body, problem)
	}

	// Input that does not start with a field has no header at all.
	in := []byte("\x00\x01binary garbage\nmore")
	h, body, _ = parseHeaderBlock(in)
	if h.Len() != 0 || !bytes.Equal(body, in) {
		t.Fatalf("no header: %d fields, body %q", h.Len(), body)
	}

	// The first of two fields with one name is what Get returns, and the
	// fields iterate in message order.
	h, _, _ = parseHeaderBlock(crlf("Received: first\nReceived: second\n\n"))
	if h.Get("Received") != "first" {
		t.Fatalf("Get = %q, want first", h.Get("Received"))
	}
}

func TestSplitMultipart(t *testing.T) {
	t.Parallel()

	body := crlf("preamble\n--b\nContent-Type: text/plain\n\none\n--b\n\ntwo\n--b--\nepilogue\n")
	pre, parts, closed, found := splitMultipart(body, "b")
	if !found || !closed || string(pre) != "preamble" || len(parts) != 2 {
		t.Fatalf("pre %q parts %d closed %v found %v", pre, len(parts), closed, found)
	}
	if string(parts[0]) != "Content-Type: text/plain\r\n\r\none" || string(parts[1]) != "\r\ntwo" {
		t.Fatalf("parts = %q", parts)
	}

	// No closing delimiter: the last part runs to the end.
	_, parts, closed, _ = splitMultipart(crlf("--b\n\none\n--b\n\ntwo, cut off"), "b")
	if closed || len(parts) != 2 || string(parts[1]) != "\r\ntwo, cut off" {
		t.Fatalf("truncated: closed %v parts %q", closed, parts)
	}

	// A delimiter that is a prefix of another boundary is not ours.
	_, parts, _, _ = splitMultipart(crlf("--b\n\none\n--b-inner\n--b--\n"), "b")
	if len(parts) != 1 || !bytes.Contains(parts[0], []byte("--b-inner")) {
		t.Fatalf("prefix: parts %q", parts)
	}

	if _, _, _, found := splitMultipart([]byte("no delimiters here"), "b"); found {
		t.Fatal("found a delimiter in a body with none")
	}
}

// Review focus 1.
func TestSplitMultipart_MixedLineEndings(t *testing.T) {
	t.Parallel()
	body := []byte("--b\r\n\r\none\n--b\n\ntwo\r\n--b--\r\n")
	_, parts, closed, _ := splitMultipart(body, "b")
	if !closed || len(parts) != 2 || string(parts[0]) != "\r\none" || string(parts[1]) != "\ntwo" {
		t.Fatalf("closed %v parts %q", closed, parts)
	}
}

// Review focus 2.
func TestSplitMultipart_BoundaryInsideALineIsContent(t *testing.T) {
	t.Parallel()
	body := crlf("--b\n\nthe text --b is not a delimiter\n--b--\n")
	_, parts, _, _ := splitMultipart(body, "b")
	if len(parts) != 1 || !bytes.Contains(parts[0], []byte("the text --b is not a delimiter")) {
		t.Fatalf("parts %q", parts)
	}
}

func TestDecodeBase64Lenient(t *testing.T) {
	t.Parallel()

	want := []byte("hello, world")
	enc := base64.StdEncoding.EncodeToString(want)
	if got, problem := decodeBase64Lenient([]byte(enc[:8] + "\r\n" + enc[8:])); !bytes.Equal(got, want) || problem != "" {
		t.Fatalf("wrapped: %q %q", got, problem)
	}
	if got, problem := decodeBase64Lenient([]byte(enc[:8] + "!!" + enc[8:])); !bytes.Equal(got, want) || problem == "" {
		t.Fatalf("junk: %q %q", got, problem)
	}
	if got, _ := decodeBase64Lenient([]byte(strings.TrimRight(enc, "="))); !bytes.Equal(got, want) {
		t.Fatalf("no padding: %q", got)
	}
	// Nothing decodable: the raw body is kept.
	if got, problem := decodeBase64Lenient([]byte("!!!!")); string(got) != "!!!!" || problem == "" {
		t.Fatalf("nothing: %q %q", got, problem)
	}
}

// Review focus 4.
func TestDecodeBase64Lenient_OneLongLine(t *testing.T) {
	t.Parallel()
	want := bytes.Repeat([]byte("0123456789"), 10_000)
	got, problem := decodeBase64Lenient([]byte(base64.StdEncoding.EncodeToString(want)))
	if !bytes.Equal(got, want) || problem != "" {
		t.Fatalf("decoded %d bytes, want %d (%q)", len(got), len(want), problem)
	}
}

func TestDecodeQuotedPrintable(t *testing.T) {
	t.Parallel()
	got := decodeQuotedPrintable([]byte("caf=C3=A9 long=\r\nline =XY broken="))
	if string(got) != "café longline =XY broken" {
		t.Fatalf("got %q", got)
	}
}

func TestDecodeText(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, typ, label string
		in               []byte
		want, kind       string
	}{
		{"utf-8", "text/plain", "utf-8", []byte("café"), "café", ""},
		{"latin-1", "text/plain", "iso-8859-1", []byte("caf\xe9"), "café", ""},
		{"unknown, not UTF-8", "text/plain", "x-nobody-knows", []byte("caf\xe9"), "café", "unknown-charset"},
		{"unknown, UTF-8", "text/plain", "x-nobody-knows", []byte("café"), "café", "unknown-charset"},
		{"none, not UTF-8", "text/plain", "", []byte("caf\xe9"), "café", "unknown-charset"},
		{"declared UTF-8, invalid", "text/plain", "utf-8", []byte("caf\xff"), "caf\uFFFD", "invalid-utf8"},
		{"html meta", "text/html", "", []byte(`<meta charset="iso-8859-1"><p>caf` + "\xe9"), `<meta charset="iso-8859-1"><p>café`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, kind, _ := decodeText(tc.typ, tc.label, tc.in)
			if string(got) != tc.want || kind != tc.kind {
				t.Fatalf("got %q kind %q; want %q kind %q", got, kind, tc.want, tc.kind)
			}
		})
	}
}

func TestReadTree(t *testing.T) {
	t.Parallel()

	raw := crlf("Content-Type: multipart/mixed; boundary=b\n\n" +
		"--b\nContent-Type: text/plain; charset=iso-8859-1\nContent-Transfer-Encoding: quoted-printable\n\ncaf=E9\n" +
		"--b\nContent-Type: application/pdf; name=doc.pdf\nContent-Disposition: inline\nContent-Transfer-Encoding: base64\n\nJVBERi0xLjQK\n" +
		"--b--\n")
	root, warnings := readTree(raw)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %+v", warnings)
	}
	if root.Type != "multipart/mixed" || len(root.Children) != 2 {
		t.Fatalf("root %s with %d children", root.Type, len(root.Children))
	}
	text, pdf := root.Children[0], root.Children[1]
	if text.Index != 1 || string(text.Body) != "café" {
		t.Fatalf("text part %d %q", text.Index, text.Body)
	}
	if pdf.Disposition != "inline" || pdf.Name != "doc.pdf" || string(pdf.Body) != "%PDF-1.4\n" {
		t.Fatalf("pdf part %+v", pdf)
	}
	if len(leaves(root)) != 2 {
		t.Fatalf("leaves = %d", len(leaves(root)))
	}
}

func TestReadTree_Salvages(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		raw      []byte
		warning  string
		check    func(t *testing.T, root *mimePart)
	}{
		{
			name:    "multipart with no delimiter becomes one text part",
			raw:     crlf("Content-Type: multipart/mixed; boundary=b\n\njust some text\n"),
			warning: "bad-multipart",
			check: func(t *testing.T, root *mimePart) {
				if root.Type != "text/plain" || !strings.Contains(string(root.Body), "just some text") {
					t.Fatalf("root %s %q", root.Type, root.Body)
				}
			},
		},
		{
			name:    "truncated multipart keeps the partial last part",
			raw:     crlf("Content-Type: multipart/mixed; boundary=b\n\n--b\n\none\n--b\n\ntwo, cut"),
			warning: "truncated",
			check: func(t *testing.T, root *mimePart) {
				if len(root.Children) != 2 || string(root.Children[1].Body) != "two, cut" {
					t.Fatalf("children %d", len(root.Children))
				}
			},
		},
		{
			name:    "unparseable Content-Type with a boundary is still a multipart",
			raw:     crlf("Content-Type: multipart/mixed; boundary=b; =broken\n\n--b\n\none\n--b--\n"),
			warning: "bad-content-type",
			check: func(t *testing.T, root *mimePart) {
				if !root.isMultipart() || len(root.Children) != 1 {
					t.Fatalf("root %s children %d", root.Type, len(root.Children))
				}
			},
		},
		{
			name:    "unknown transfer encoding keeps the bytes",
			raw:     crlf("Content-Transfer-Encoding: x-uuencode\n\nbegin 644 x\n"),
			warning: "bad-encoding",
			check: func(t *testing.T, root *mimePart) {
				if !strings.Contains(string(root.Body), "begin 644 x") {
					t.Fatalf("body %q", root.Body)
				}
			},
		},
		{
			name:    "a multipart with only a preamble shows the preamble",
			raw:     crlf("Content-Type: multipart/mixed; boundary=b\n\nthe preamble\n--b--\n"),
			warning: "",
			check: func(t *testing.T, root *mimePart) {
				if len(root.Children) != 1 || string(root.Children[0].Body) != "the preamble" {
					t.Fatalf("children %+v", root.Children)
				}
			},
		},
		{
			name:    "children of a digest default to message/rfc822",
			raw:     crlf("Content-Type: multipart/digest; boundary=b\n\n--b\n\nSubject: inner\n\nhi\n--b--\n"),
			warning: "",
			check: func(t *testing.T, root *mimePart) {
				if root.Children[0].Type != "message/rfc822" {
					t.Fatalf("child type %s", root.Children[0].Type)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, warnings := readTree(tc.raw)
			if tc.warning != "" && !hasWarning(warnings, tc.warning) {
				t.Fatalf("warnings %+v, want %s", warnings, tc.warning)
			}
			tc.check(t, root)
		})
	}
}

func TestReadTree_TooDeep(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	for i := 0; i < maxMIMEDepth+5; i++ {
		b.WriteString("Content-Type: multipart/mixed; boundary=b" + strings.Repeat("x", i) + "\r\n\r\n--b" + strings.Repeat("x", i) + "\r\n")
	}
	b.WriteString("\r\ndeep text\r\n")
	root, warnings := readTree([]byte(b.String()))
	if !hasWarning(warnings, "too-deep") {
		t.Fatalf("warnings %+v", warnings)
	}
	found := false
	for _, leaf := range leaves(root) {
		if bytes.Contains(leaf.Body, []byte("deep text")) {
			found = true
		}
	}
	if !found {
		t.Fatal("the deep text was lost")
	}
}

// Review focus 3.
func TestReadTree_UndecodableFilenameIsKept(t *testing.T) {
	t.Parallel()
	raw := crlf("Content-Type: application/pdf\nContent-Disposition: attachment; filename=\"=?x-nobody-knows?Q?r=E9sum=E9?=.pdf\"\n\nPDF")
	root, warnings := readTree(raw)
	if root.Name == "" || !strings.HasSuffix(root.Name, ".pdf") || string(root.Body) != "PDF" {
		t.Fatalf("part %+v", root)
	}
	if !hasWarning(warnings, "bad-header-param") {
		t.Fatalf("warnings %+v", warnings)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test -run 'TestParseHeaderBlock|TestSplitMultipart|TestDecode|TestReadTree' .`
Expected: FAIL to compile, `undefined: parseHeaderBlock` and the others.

- [ ] **Step 3: Implement**

Create `mail/server/mimetree.go`:

```go
package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/textproto"
)

// readTree salvages a message's whole MIME tree from any input (design,
// "Step 1"). It never fails and never drops bytes it could keep: what it
// cannot decode it keeps undecoded, and every problem is a warning.
//
// It parses headers, boundaries and encodings itself instead of using
// go-message's entity reader, which refuses a malformed header line and
// decodes transfer encodings and charsets with no way to keep a partial
// result. It still fills a go-message textproto.Header, so go-message's
// mail.Header helpers read the fields.

// maxMIMEDepth bounds how deep readTree follows nested multiparts, so a
// crafted message cannot exhaust the stack.
const maxMIMEDepth = 50

type mimePart struct {
	Index       int
	Header      textproto.Header
	Type        string
	Params      map[string]string
	Disposition string
	Name        string
	ContentID   string
	Body        []byte
	RawSize     int
	Children    []*mimePart
}

func (p *mimePart) isMultipart() bool { return strings.HasPrefix(p.Type, "multipart/") }

type parseWarning struct {
	Part   int
	Kind   string
	Detail string
}

func readTree(raw []byte) (*mimePart, []parseWarning) {
	r := &treeReader{}
	return r.read(raw, 0, "text/plain"), r.warnings
}

// leaves returns the tree's non-multipart parts, depth-first.
func leaves(root *mimePart) []*mimePart {
	if !root.isMultipart() {
		return []*mimePart{root}
	}
	var out []*mimePart
	for _, c := range root.Children {
		out = append(out, leaves(c)...)
	}
	return out
}

type treeReader struct {
	next     int
	warnings []parseWarning
}

func (r *treeReader) warn(part int, kind, format string, args ...any) {
	r.warnings = append(r.warnings, parseWarning{Part: part, Kind: kind, Detail: fmt.Sprintf(format, args...)})
}

// read builds the part whose header and body are b. defaultType is the type
// of a part with no Content-Type: text/plain, or message/rfc822 inside a
// multipart/digest (RFC 2046 §5.1.5).
func (r *treeReader) read(b []byte, depth int, defaultType string) *mimePart {
	p := &mimePart{Index: r.next}
	r.next++

	h, body, problem := parseHeaderBlock(b)
	if problem != "" {
		r.warn(p.Index, "bad-header", "%s", problem)
	}
	p.Header = h
	p.RawSize = len(bytes.TrimSpace(body))

	ct := p.Header.Get("Content-Type")
	typ, params, ok := parseContentType(ct, defaultType)
	if !ok {
		r.warn(p.Index, "bad-content-type", "Content-Type of %d bytes read as %s", len(ct), typ)
	}
	p.Type, p.Params = typ, params
	p.Disposition, p.Name = r.disposition(p)
	p.ContentID = strings.TrimSpace(p.Header.Get("Content-Id"))

	if p.isMultipart() {
		r.readChildren(p, body, depth)
	} else {
		r.decodeLeaf(p, p.Header.Get("Content-Transfer-Encoding"), body)
	}
	return p
}

func (r *treeReader) readChildren(p *mimePart, body []byte, depth int) {
	if depth >= maxMIMEDepth {
		r.warn(p.Index, "too-deep", "multipart nested deeper than %d levels; kept as one file", maxMIMEDepth)
		r.keepAsLeaf(p, "application/octet-stream", body)
		return
	}
	preamble, parts, closed, found := splitMultipart(body, p.Params["boundary"])
	if !found {
		r.warn(p.Index, "bad-multipart", "no delimiter for boundary %q; kept as one part", p.Params["boundary"])
		typ := "application/octet-stream"
		if looksLikeText(body) {
			typ = "text/plain"
		}
		r.keepAsLeaf(p, typ, body)
		return
	}
	if !closed {
		r.warn(p.Index, "truncated", "no closing delimiter; the last part runs to the end of the message")
	}
	if len(parts) == 0 && len(bytes.TrimSpace(preamble)) > 0 {
		// RFC 2046 drops the preamble, but a multipart with no parts has
		// nothing else to show.
		child := &mimePart{Index: r.next, Type: "text/plain", Params: map[string]string{}, RawSize: len(bytes.TrimSpace(preamble))}
		r.next++
		r.decodeLeaf(child, "", preamble)
		p.Children = []*mimePart{child}
		return
	}
	childType := "text/plain"
	if p.Type == "multipart/digest" {
		childType = "message/rfc822"
	}
	for _, raw := range parts {
		p.Children = append(p.Children, r.read(raw, depth+1, childType))
	}
}

// keepAsLeaf turns a multipart that cannot be split into one leaf holding
// its raw bytes.
func (r *treeReader) keepAsLeaf(p *mimePart, typ string, body []byte) {
	p.Type = typ
	p.Params = map[string]string{}
	p.Children = nil
	r.decodeLeaf(p, "", body)
}

func (r *treeReader) decodeLeaf(p *mimePart, cte string, body []byte) {
	decoded, problem := decodeTransfer(cte, body)
	if problem != "" {
		r.warn(p.Index, "bad-encoding", "%s", problem)
	}
	if strings.HasPrefix(p.Type, "text/") {
		text, kind, problem := decodeText(p.Type, p.Params["charset"], decoded)
		if problem != "" {
			r.warn(p.Index, kind, "%s", problem)
		}
		decoded = text
	}
	p.Body = decoded
}

func (r *treeReader) disposition(p *mimePart) (string, string) {
	raw := p.Header.Get("Content-Disposition")
	disp, params := "", map[string]string{}
	if strings.TrimSpace(raw) != "" {
		d, ps, err := mime.ParseMediaType(raw)
		if err != nil {
			d, ps = lenientMediaType(raw)
			r.warn(p.Index, "bad-header-param", "Content-Disposition of %d bytes read leniently", len(raw))
		}
		disp, params = strings.ToLower(d), ps
		if disp != "inline" && disp != "attachment" {
			// RFC 2183: an unknown disposition is treated as attachment.
			disp = "attachment"
		}
	}
	name := params["filename"]
	if name == "" {
		name = p.Params["name"]
	}
	return disp, r.decodeWords(p.Index, name)
}

var wordDecoder = mime.WordDecoder{CharsetReader: charset.Reader}

// decodeWords decodes RFC 2047 encoded words, keeping the value as written
// when it cannot.
func (r *treeReader) decodeWords(part int, v string) string {
	if !strings.Contains(v, "=?") {
		return v
	}
	out, err := wordDecoder.DecodeHeader(v)
	if err != nil {
		r.warn(part, "bad-header-param", "could not decode a %d-byte parameter: %v", len(v), err)
		return v
	}
	return out
}

var headerFieldLine = regexp.MustCompile(`^([!-9;-~]+)[ \t]*:(.*)$`)

// parseHeaderBlock reads the header fields at the start of b and returns
// them with the body after the blank line that ends them. It never fails. A
// line that is neither a field nor a continuation is skipped when a field
// follows it, and otherwise ends the header there: the body then starts at
// that line, so nothing is lost. Input whose first line is not a field has no
// header and is all body.
func parseHeaderBlock(b []byte) (textproto.Header, []byte, string) {
	var fields []headerLine
	var problem string
	for pos := 0; pos < len(b); {
		line, next := lineAt(b, pos)
		switch {
		case len(line) == 0:
			return headerOf(fields), b[next:], problem
		case line[0] == ' ' || line[0] == '\t':
			if len(fields) == 0 {
				return textproto.Header{}, b, ""
			}
			// RFC 5322 unfolding removes the line break and keeps the white space.
			fields[len(fields)-1].value += string(line)
		default:
			if m := headerFieldLine.FindSubmatch(line); m != nil {
				fields = append(fields, headerLine{string(m[1]), string(m[2])})
				break
			}
			if pos == 0 {
				return textproto.Header{}, b, ""
			}
			// Problems name a line's length, never its text: a header line
			// can hold a name or an address, and warnings reach the log.
			if next < len(b) {
				if following, _ := lineAt(b, next); headerFieldLine.Match(following) {
					problem = fmt.Sprintf("skipped a header line of %d bytes that is not a field", len(line))
					break
				}
			}
			return headerOf(fields), b[pos:], fmt.Sprintf("header ended early at a line of %d bytes that is not a field", len(line))
		}
		pos = next
	}
	return headerOf(fields), nil, problem
}

type headerLine struct{ name, value string }

// headerOf builds a go-message header from fields in message order. The
// header stores fields last-first, so they are added in reverse: Get then
// returns the first field of a name, and Fields iterates in message order,
// as go-message's own reader leaves them.
func headerOf(fields []headerLine) textproto.Header {
	var h textproto.Header
	for i := len(fields) - 1; i >= 0; i-- {
		h.Add(strings.TrimSpace(fields[i].name), strings.TrimSpace(fields[i].value))
	}
	return h
}

// lineAt returns the line at pos without its line break, and where the next
// line starts.
func lineAt(b []byte, pos int) ([]byte, int) {
	end := bytes.IndexByte(b[pos:], '\n')
	if end < 0 {
		return bytes.TrimSuffix(b[pos:], []byte("\r")), len(b)
	}
	return bytes.TrimSuffix(b[pos:pos+end], []byte("\r")), pos + end + 1
}

// parseContentType reads a Content-Type value. ok is false when it had to
// be read leniently; a value with no usable type falls back to defaultType,
// keeping whatever parameters (a boundary) could be read.
func parseContentType(v, defaultType string) (string, map[string]string, bool) {
	if strings.TrimSpace(v) == "" {
		return defaultType, map[string]string{}, true
	}
	if mt, params, err := mime.ParseMediaType(v); err == nil {
		return mt, params, true
	}
	mt, params := lenientMediaType(v)
	if !strings.Contains(mt, "/") {
		return defaultType, params, false
	}
	return mt, params, false
}

var lenientParam = regexp.MustCompile(`(?i)([a-z0-9!#$&^_.+-]+)(\*[0-9]*\*?)?\s*=\s*("(?:[^"\\]|\\.)*"?|[^;\s]*)`)

// lenientMediaType reads a Content-Type or Content-Disposition value that
// mime.ParseMediaType refuses: the type is the text before the first ';' and
// each name=value pair after it is a parameter. RFC 2231 continuations
// (name*0, name*1) are joined and extended values (name*=utf-8''…) are
// percent-decoded; a value that cannot be decoded stays as written.
func lenientMediaType(v string) (string, map[string]string) {
	typ, rest, _ := strings.Cut(v, ";")
	params := map[string]string{}
	pieces := map[string][]string{}
	for _, m := range lenientParam.FindAllStringSubmatch(rest, -1) {
		name, ext, value := strings.ToLower(m[1]), m[2], unquoteParam(m[3])
		if strings.HasSuffix(ext, "*") {
			value = decodeExtValue(value)
		}
		if ext != "" && ext != "*" {
			pieces[name] = append(pieces[name], value)
			continue
		}
		params[name] = value
	}
	for name, ps := range pieces {
		if _, ok := params[name]; !ok {
			params[name] = strings.Join(ps, "")
		}
	}
	return strings.ToLower(strings.TrimSpace(typ)), params
}

func unquoteParam(v string) string {
	if strings.HasPrefix(v, `"`) {
		v = strings.TrimSuffix(strings.TrimPrefix(v, `"`), `"`)
		v = strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(v)
	}
	return strings.TrimSpace(v)
}

// decodeExtValue decodes an RFC 2231 extended value, charset'lang'%XX…, or
// a continuation piece that is only %XX…, keeping it as written on failure.
func decodeExtValue(v string) string {
	cs, encoded := "", v
	if parts := strings.SplitN(v, "'", 3); len(parts) == 3 {
		cs, encoded = parts[0], parts[2]
	}
	b, err := url.PathUnescape(encoded)
	if err != nil {
		return v
	}
	if cs == "" || strings.EqualFold(cs, "utf-8") || strings.EqualFold(cs, "us-ascii") {
		return b
	}
	rd, err := charset.Reader(cs, strings.NewReader(b))
	if err != nil {
		return b
	}
	out, err := io.ReadAll(rd)
	if err != nil {
		return b
	}
	return string(out)
}

// splitMultipart cuts a multipart body at the delimiter lines of boundary. A
// delimiter line is "--boundary", or "--boundary--" for the close, at the
// start of a line, with only white space after it; the line break before it
// belongs to the delimiter (RFC 2046 §5.1.1). found is false when no
// delimiter line exists. closed is false when the close delimiter is missing;
// the last part then runs to the end of body.
func splitMultipart(body []byte, boundary string) (preamble []byte, parts [][]byte, closed, found bool) {
	if boundary == "" {
		return nil, nil, false, false
	}
	delim := []byte("--" + boundary)
	start := -1
	for pos := 0; pos < len(body); {
		line, next := lineAt(body, pos)
		if kind := delimiterKind(line, delim); kind != 0 {
			found = true
			cut := lineBreakStart(body, pos)
			if start < 0 {
				preamble = body[:cut]
			} else {
				parts = append(parts, body[start:max(cut, start)])
			}
			if kind == 2 {
				return preamble, parts, true, true
			}
			start = next
		}
		pos = next
	}
	if start >= 0 && start < len(body) {
		parts = append(parts, body[start:])
	}
	return preamble, parts, false, found
}

// delimiterKind is 1 for an open delimiter line, 2 for the close, 0 otherwise.
func delimiterKind(line, delim []byte) int {
	line = bytes.TrimRight(line, " \t\r")
	if !bytes.HasPrefix(line, delim) {
		return 0
	}
	switch rest := line[len(delim):]; {
	case len(rest) == 0:
		return 1
	case bytes.Equal(rest, []byte("--")):
		return 2
	}
	return 0
}

// lineBreakStart is where the line break before the line at pos begins.
func lineBreakStart(b []byte, pos int) int {
	switch {
	case pos >= 2 && b[pos-2] == '\r' && b[pos-1] == '\n':
		return pos - 2
	case pos >= 1 && b[pos-1] == '\n':
		return pos - 1
	}
	return pos
}

// looksLikeText reports whether b reads as text: at most 1% control bytes in
// its first 4 KB.
func looksLikeText(b []byte) bool {
	sample := b[:min(len(b), 4096)]
	control := 0
	for _, c := range sample {
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' && c != '\f' {
			control++
		}
	}
	return control*100 <= len(sample)
}

func decodeTransfer(cte string, body []byte) ([]byte, string) {
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "", "7bit", "8bit", "binary":
		return body, ""
	case "base64":
		return decodeBase64Lenient(body)
	case "quoted-printable":
		return decodeQuotedPrintable(body), ""
	default:
		return body, fmt.Sprintf("unknown Content-Transfer-Encoding %q; kept as is", cte)
	}
}

// decodeBase64Lenient decodes base64 that may carry line breaks, stray
// characters or broken padding, as lenient mail readers do: it keeps only
// the base64 alphabet, decodes each run between padding, and drops a
// trailing character too short to hold a byte. With nothing decodable the
// raw body is kept.
func decodeBase64Lenient(body []byte) ([]byte, string) {
	var out, run []byte
	junk := 0
	flush := func() {
		if len(run)%4 == 1 {
			run = run[:len(run)-1]
		}
		if len(run) > 0 {
			dst := make([]byte, base64.RawStdEncoding.DecodedLen(len(run)))
			n, _ := base64.RawStdEncoding.Decode(dst, run)
			out = append(out, dst[:n]...)
		}
		run = run[:0]
	}
	for _, c := range body {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '+', c == '/':
			run = append(run, c)
		case c == '=':
			flush()
		case c == ' ', c == '\t', c == '\r', c == '\n':
		default:
			junk++
		}
	}
	flush()
	if len(out) == 0 && len(bytes.TrimSpace(body)) > 0 {
		return body, "base64 body held nothing decodable; kept as is"
	}
	if junk > 0 {
		return out, fmt.Sprintf("dropped %d bytes outside the base64 alphabet", junk)
	}
	return out, ""
}

// decodeQuotedPrintable decodes quoted-printable leniently: "=XX" with two
// hex digits is a byte, "=" before a line break is a soft break, and any
// other "=" stays as written, so broken encoding never loses text.
func decodeQuotedPrintable(body []byte) []byte {
	out := make([]byte, 0, len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '=' {
			out = append(out, c)
			continue
		}
		if i+2 < len(body) && isHex(body[i+1]) && isHex(body[i+2]) {
			out = append(out, unhex(body[i+1])<<4|unhex(body[i+2]))
			i += 2
			continue
		}
		j := i + 1
		for j < len(body) && (body[j] == ' ' || body[j] == '\t') {
			j++
		}
		switch {
		case j == len(body):
			i = j - 1
		case body[j] == '\n':
			i = j
		case body[j] == '\r' && j+1 < len(body) && body[j+1] == '\n':
			i = j + 1
		default:
			out = append(out, c)
		}
	}
	return out
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f'
}

func unhex(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}

var metaCharset = regexp.MustCompile(`(?i)<meta[^>]+charset\s*=\s*["']?\s*([a-z0-9_.:-]+)`)

// decodeText turns a text part's bytes into UTF-8. A declared charset that
// go-message's charset package knows is used. Otherwise, and when none is
// declared (for HTML, after a <meta charset>), the bytes are used as they are
// when they are UTF-8 and read as Windows-1252 when not, which maps every
// byte to a character. kind and problem say what had to be guessed.
func decodeText(typ, label string, b []byte) ([]byte, string, string) {
	label = strings.ToLower(strings.TrimSpace(label))
	if label == "" && typ == "text/html" {
		if m := metaCharset.FindSubmatch(b[:min(len(b), 2048)]); m != nil {
			label = strings.ToLower(string(m[1]))
		}
	}
	switch label {
	case "", "us-ascii", "ascii":
		if utf8.Valid(b) {
			return b, "", ""
		}
		return windows1252(b), "unknown-charset", fmt.Sprintf("charset %q but not UTF-8; read as Windows-1252", label)
	case "utf-8", "utf8":
		if utf8.Valid(b) {
			return b, "", ""
		}
		return bytes.ToValidUTF8(b, []byte("\uFFFD")), "invalid-utf8", "declared UTF-8 but holds invalid sequences; replaced them with U+FFFD"
	}
	if rd, err := charset.Reader(label, bytes.NewReader(b)); err == nil {
		if decoded, err := io.ReadAll(rd); err == nil {
			if utf8.Valid(decoded) {
				return decoded, "", ""
			}
			return bytes.ToValidUTF8(decoded, []byte("\uFFFD")), "invalid-utf8", fmt.Sprintf("%s text decoded to invalid UTF-8; replaced the bad sequences", label)
		}
	}
	if utf8.Valid(b) {
		return b, "unknown-charset", fmt.Sprintf("unknown charset %q; the bytes are UTF-8 and were used as such", label)
	}
	return windows1252(b), "unknown-charset", fmt.Sprintf("unknown charset %q; read as Windows-1252", label)
}

func windows1252(b []byte) []byte {
	if rd, err := charset.Reader("windows-1252", bytes.NewReader(b)); err == nil {
		if out, err := io.ReadAll(rd); err == nil {
			return bytes.ToValidUTF8(out, []byte("\uFFFD"))
		}
	}
	return bytes.ToValidUTF8(b, []byte("\uFFFD"))
}
```

Warning details never quote a header value, only field names, lengths and charset names: a header value can hold a name or an address, and Task 8 writes the details to the log.

- [ ] **Step 4: Run the tests**

Run: `GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test -run 'TestParseHeaderBlock|TestSplitMultipart|TestDecode|TestReadTree' -v . 2>&1 | grep -E '^(--- |ok|FAIL)'`
Expected: every test PASS. A failing case is a bug in `mimetree.go`; fix it there, not in the test.

- [ ] **Step 5: Commit**

```bash
cd ~/code/tinycld/mail
gofmt -l server
git add server/mimetree.go server/mimetree_test.go
git commit -m "feat(mail): read a salvaged MIME tree from any message"
```

---

### Task 3: `parseStructure` and `flatten`

**Files:**
- Create: `mail/server/bodystructure.go`
- Test: `mail/server/bodystructure_test.go`

**Interfaces:**
- Consumes: `mimePart`, `leaves`, `readTree` (Task 2); `stripHTMLToText` (`sanitize.go`); `newInboundAttachment` (`imap_rfc5322.go`); `normalizeCID` (`cid_rewrite.go`).
- Produces:

```go
type bodyLists struct{ text, html, attachments []*mimePart }
func parseStructure(root *mimePart) bodyLists
func flatten(l bodyLists, msg *storedMessage)
```

- [ ] **Step 1: Write the failing tests**

Create `mail/server/bodystructure_test.go`:

```go
package mail

import (
	"slices"
	"strings"
	"testing"
)

// indexes renders a list as part indexes, for compact assertions.
func indexes(ps []*mimePart) []int {
	out := make([]int, len(ps))
	for i, p := range ps {
		out[i] = p.Index
	}
	return out
}

func structureOf(t *testing.T, raw string) bodyLists {
	t.Helper()
	root, _ := readTree(crlf(raw))
	return parseStructure(root)
}

func TestParseStructure_RFC8621(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name              string
		raw               string
		text, html, files []int
	}{
		{
			name: "single text part",
			raw:  "Content-Type: text/plain\n\nhi\n",
			text: []int{0}, html: []int{0}, files: []int{},
		},
		{
			name: "alternative picks plain for text and html for html",
			raw: "Content-Type: multipart/alternative; boundary=a\n\n" +
				"--a\nContent-Type: text/plain\n\nhi\n--a\nContent-Type: text/html\n\n<p>hi</p>\n--a--\n",
			text: []int{1}, html: []int{2}, files: []int{},
		},
		{
			name: "html only in an alternative fills the text list too",
			raw: "Content-Type: multipart/alternative; boundary=a\n\n" +
				"--a\nContent-Type: text/html\n\n<p>hi</p>\n--a--\n",
			text: []int{1}, html: []int{1}, files: []int{},
		},
		{
			name: "mixed: body then an attachment",
			raw: "Content-Type: multipart/mixed; boundary=m\n\n" +
				"--m\nContent-Type: text/plain\n\nhi\n--m\nContent-Type: application/pdf\nContent-Disposition: attachment; filename=a.pdf\n\nPDF\n--m--\n",
			text: []int{1}, html: []int{1}, files: []int{2},
		},
		{
			name: "html split around an inline PDF inside an alternative",
			raw: "Content-Type: multipart/alternative; boundary=a\n\n" +
				"--a\nContent-Type: text/plain\n\nAttach pdf here\n" +
				"--a\nContent-Type: multipart/mixed; boundary=m\n\n" +
				"--m\nContent-Type: text/html\n\n<p>Attach pdf here</p>\n" +
				"--m\nContent-Type: application/pdf; name=doc.pdf\nContent-Disposition: inline\n\nPDF\n" +
				"--m\nContent-Type: text/html\n\n<p></p>\n--m--\n--a--\n",
			text: []int{1}, html: []int{3, 5}, files: []int{4},
		},
		{
			name: "inline image in mixed is in both bodies and not a file",
			raw: "Content-Type: multipart/mixed; boundary=m\n\n" +
				"--m\nContent-Type: text/plain\n\nlook\n--m\nContent-Type: image/png\n\nPNG\n--m--\n",
			text: []int{1, 2}, html: []int{1, 2}, files: []int{},
		},
		{
			name: "related: only the first part is a body",
			raw: "Content-Type: multipart/related; boundary=r\n\n" +
				"--r\nContent-Type: text/html\n\n<img src=\"cid:x\">\n--r\nContent-Type: image/png\nContent-Id: <x>\n\nPNG\n--r--\n",
			text: []int{1}, html: []int{1}, files: []int{2},
		},
		{
			name: "files only, no text",
			raw: "Content-Type: multipart/mixed; boundary=m\n\n" +
				"--m\nContent-Type: application/octet-stream\nContent-Disposition: attachment; filename=a.bin\n\nBIN\n" +
				"--m\nContent-Type: application/pdf\nContent-Disposition: inline; filename=b.pdf\n\nPDF\n--m--\n",
			text: []int{}, html: []int{}, files: []int{1, 2},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := structureOf(t, tc.raw)
			if got := indexes(l.text); !slices.Equal(got, tc.text) {
				t.Errorf("text = %v, want %v", got, tc.text)
			}
			if got := indexes(l.html); !slices.Equal(got, tc.html) {
				t.Errorf("html = %v, want %v", got, tc.html)
			}
			if got := indexes(l.attachments); !slices.Equal(got, tc.files) {
				t.Errorf("attachments = %v, want %v", got, tc.files)
			}
		})
	}
}

// RFC 8621 defines attachments as every non-multipart part in neither body
// list, but the algorithm can drop one: inside an alternative, a mixed
// branch whose HTML part stops its text list and whose later plain part then
// stops its HTML list puts that plain part nowhere. It is added back.
func TestParseStructure_NoLeafIsDropped(t *testing.T) {
	t.Parallel()
	l := structureOf(t, "Content-Type: multipart/alternative; boundary=a\n\n"+
		"--a\nContent-Type: text/plain\n\nhi\n"+
		"--a\nContent-Type: multipart/mixed; boundary=m\n\n"+
		"--m\nContent-Type: text/html\n\n<p>hi</p>\n--m\nContent-Type: text/plain\n\nfooter\n--m--\n--a--\n")
	in := map[int]bool{}
	for _, list := range [][]*mimePart{l.text, l.html, l.attachments} {
		for _, p := range list {
			in[p.Index] = true
		}
	}
	for _, i := range []int{1, 3, 4} {
		if !in[i] {
			t.Fatalf("part %d is in no list: text %v html %v files %v", i, indexes(l.text), indexes(l.html), indexes(l.attachments))
		}
	}
}

func flattened(t *testing.T, raw string) *storedMessage {
	t.Helper()
	msg := &storedMessage{}
	flatten(structureOf(t, raw), msg)
	return msg
}

func TestFlatten(t *testing.T) {
	t.Parallel()

	msg := flattened(t, "Content-Type: multipart/alternative; boundary=a\n\n"+
		"--a\nContent-Type: text/plain\n\nAttach pdf here\n"+
		"--a\nContent-Type: multipart/mixed; boundary=m\n\n"+
		"--m\nContent-Type: text/html\n\n<p>Attach pdf here</p>\n"+
		"--m\nContent-Type: application/pdf; name=doc.pdf\nContent-Disposition: inline\n\nPDF\n"+
		"--m\nContent-Type: text/html\n\n<p>after</p>\n--m--\n--a--\n")
	if msg.TextBody != "Attach pdf here" {
		t.Errorf("text = %q", msg.TextBody)
	}
	if !strings.Contains(msg.HTMLBody, "Attach pdf here") || !strings.Contains(msg.HTMLBody, "after") {
		t.Errorf("html = %q", msg.HTMLBody)
	}
	if len(msg.Attachments) != 1 || msg.Attachments[0].Name != "doc.pdf" {
		t.Errorf("attachments = %+v", msg.Attachments)
	}

	// A plain-text message stays plain text.
	msg = flattened(t, "Content-Type: text/plain\n\nhello\n")
	if msg.TextBody != "hello" || msg.HTMLBody != "" {
		t.Errorf("plain: text %q html %q", msg.TextBody, msg.HTMLBody)
	}

	// An image between two HTML parts shows where it was put, and is a file.
	msg = flattened(t, "Content-Type: multipart/mixed; boundary=m\n\n"+
		"--m\nContent-Type: text/html\n\n<p>one</p>\n--m\nContent-Type: image/png\n\nPNG\n"+
		"--m\nContent-Type: text/html\n\n<p>two</p>\n--m--\n")
	one, img, two := strings.Index(msg.HTMLBody, "one"), strings.Index(msg.HTMLBody, `<img src="cid:`), strings.Index(msg.HTMLBody, "two")
	if !(one >= 0 && one < img && img < two) {
		t.Errorf("html order: %q", msg.HTMLBody)
	}
	if len(msg.Attachments) != 1 || msg.Attachments[0].ContentID == "" {
		t.Errorf("image file: %+v", msg.Attachments)
	}

	// Files and no text: both bodies empty, every file kept.
	msg = flattened(t, "Content-Type: multipart/mixed; boundary=m\n\n"+
		"--m\nContent-Type: application/octet-stream\nContent-Disposition: attachment; filename=a.bin\n\nBIN\n"+
		"--m\nContent-Type: application/pdf\nContent-Disposition: inline; filename=b.pdf\n\nPDF\n--m--\n")
	if msg.TextBody != "" || msg.HTMLBody != "" || len(msg.Attachments) != 2 {
		t.Errorf("files only: text %q html %q files %d", msg.TextBody, msg.HTMLBody, len(msg.Attachments))
	}

	// An attached message without a name is named message.eml.
	msg = flattened(t, "Content-Type: multipart/mixed; boundary=m\n\n"+
		"--m\nContent-Type: text/plain\n\nfwd\n--m\nContent-Type: message/rfc822\n\nSubject: inner\n\nhi\n--m--\n")
	if len(msg.Attachments) != 1 || msg.Attachments[0].Name != "message.eml" {
		t.Errorf("attached message: %+v", msg.Attachments)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test -run 'TestParseStructure|TestFlatten' .`
Expected: FAIL to compile, `undefined: parseStructure`.

- [ ] **Step 3: Implement**

Create `mail/server/bodystructure.go`:

```go
package mail

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
)

// The rules that pick a message's text body, HTML body and files are RFC
// 8621 §4.1.4's parseStructure, ported as written (design, "Step 2"). The RFC
// calls it "a starting point … likely to require workarounds for malformed
// content". Every workaround added here names the corpus message that fails
// without it (testdata/corpus), so a workaround can be told from a bug.
//
// Workarounds:
//   - (none yet)

type bodyLists struct {
	text, html, attachments []*mimePart
}

func isInlineMediaType(t string) bool {
	return strings.HasPrefix(t, "image/") || strings.HasPrefix(t, "audio/") || strings.HasPrefix(t, "video/")
}

func parseStructure(root *mimePart) bodyLists {
	text, html, attachments := []*mimePart{}, []*mimePart{}, []*mimePart{}
	structure([]*mimePart{root}, "mixed", false, &html, &text, &attachments)
	l := bodyLists{text: text, html: html, attachments: attachments}
	l.attachments = backfill(root, l)
	return l
}

// structure is the RFC's parseStructure. A nil list pointer is the RFC's
// null: that branch stops adding to it, and only within this call, as the
// JavaScript's local parameters do.
func structure(parts []*mimePart, multipartType string, inAlternative bool, html, text, attachments *[]*mimePart) {
	textLength, htmlLength := -1, -1
	if text != nil {
		textLength = len(*text)
	}
	if html != nil {
		htmlLength = len(*html)
	}
	for i, part := range parts {
		isInline := part.Disposition != "attachment" &&
			(part.Type == "text/plain" || part.Type == "text/html" || isInlineMediaType(part.Type)) &&
			(i == 0 || (multipartType != "related" && (isInlineMediaType(part.Type) || part.Name == "")))
		switch {
		case part.isMultipart():
			sub := strings.TrimPrefix(part.Type, "multipart/")
			structure(part.Children, sub, inAlternative || sub == "alternative", html, text, attachments)
		case isInline:
			if multipartType == "alternative" {
				switch {
				case part.Type == "text/plain" && text != nil:
					*text = append(*text, part)
				case part.Type == "text/html" && html != nil:
					*html = append(*html, part)
				default:
					*attachments = append(*attachments, part)
				}
				continue
			} else if inAlternative {
				if part.Type == "text/plain" {
					html = nil
				}
				if part.Type == "text/html" {
					text = nil
				}
			}
			if text != nil {
				*text = append(*text, part)
			}
			if html != nil {
				*html = append(*html, part)
			}
			if (text == nil || html == nil) && isInlineMediaType(part.Type) {
				*attachments = append(*attachments, part)
			}
		default:
			*attachments = append(*attachments, part)
		}
	}
	if multipartType == "alternative" && text != nil && html != nil {
		// Found HTML only: it is the text body too.
		if textLength == len(*text) && htmlLength != len(*html) {
			*text = append(*text, (*html)[htmlLength:]...)
		}
		// Found plain text only: it is the HTML body too.
		if htmlLength == len(*html) && textLength != len(*text) {
			*html = append(*html, (*text)[textLength:]...)
		}
	}
}

// backfill adds every leaf the algorithm put in no list to attachments, in
// depth-first order. RFC 8621 defines attachments as every non-multipart
// part in neither body list, and the algorithm can still drop a part (a
// third alternative that is neither text nor HTML, say): a dropped part is
// lost mail.
func backfill(root *mimePart, l bodyLists) []*mimePart {
	listed := map[*mimePart]bool{}
	for _, list := range [][]*mimePart{l.text, l.html, l.attachments} {
		for _, p := range list {
			listed[p] = true
		}
	}
	out := append([]*mimePart{}, l.attachments...)
	for _, leaf := range leaves(root) {
		if !listed[leaf] {
			out = append(out, leaf)
			listed[leaf] = true
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

var cidRef = regexp.MustCompile(`(?i)cid:([^"'\s>)]+)`)

// flatten maps the three lists onto storedMessage, which holds one text, one
// HTML and a file list (design, "Step 3"). Empty parts are skipped.
func flatten(l bodyLists, msg *storedMessage) {
	var texts []string
	for _, p := range l.text {
		switch {
		case len(p.Body) == 0 || isInlineMediaType(p.Type):
		case p.Type == "text/html":
			texts = append(texts, stripHTMLToText(string(p.Body)))
		default:
			texts = append(texts, string(p.Body))
		}
	}
	msg.TextBody = strings.Join(texts, "\n")

	referenced := map[string]bool{}
	for _, p := range l.html {
		if p.Type == "text/html" {
			for _, m := range cidRef.FindAllStringSubmatch(string(p.Body), -1) {
				referenced[normalizeCID(m[1])] = true
			}
		}
	}

	var files []*mimePart
	seen := map[*mimePart]bool{}
	addFile := func(p *mimePart) {
		if !seen[p] && len(p.Body) > 0 {
			seen[p] = true
			files = append(files, p)
		}
	}

	var b strings.Builder
	hasHTML := false
	for _, p := range l.html {
		switch {
		case len(p.Body) == 0:
		case p.Type == "text/html":
			hasHTML = true
			b.Write(p.Body)
		case p.Type == "text/plain":
			b.WriteString(`<div style="white-space: pre-wrap">` + html.EscapeString(string(p.Body)) + `</div>`)
		case isInlineMediaType(p.Type):
			if p.ContentID == "" {
				p.ContentID = fmt.Sprintf("<part-%d@tinycld.invalid>", p.Index)
			}
			if cid := normalizeCID(p.ContentID); !referenced[cid] {
				// Shown where the sender put it, between the HTML parts.
				b.WriteString(`<img src="cid:` + html.EscapeString(cid) + `">`)
			}
			addFile(p)
		}
	}
	if hasHTML {
		msg.HTMLBody = b.String()
	}

	for _, p := range l.text {
		if isInlineMediaType(p.Type) {
			addFile(p)
		}
	}
	for _, p := range l.attachments {
		addFile(p)
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].Index < files[j].Index })
	for _, p := range files {
		name := p.Name
		if name == "" && p.Type == "message/rfc822" {
			name = "message.eml"
		}
		msg.Attachments = append(msg.Attachments, newInboundAttachment(name, p.Type, p.ContentID, p.Body))
	}

	if strings.TrimSpace(msg.TextBody) == "" && msg.HTMLBody != "" {
		msg.TextBody = stripHTMLToText(msg.HTMLBody)
	}
}
```

- [ ] **Step 4: Run the tests**

Run: `GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test -run 'TestParseStructure|TestFlatten|TestReadTree' -v . 2>&1 | grep -E '^(--- |ok|FAIL)'`
Expected: PASS. In `TestFlatten`, `msg.TextBody` is `"Attach pdf here"`: the text part's body excludes the line break before the delimiter, as RFC 2046 says. If the test shows a trailing `"\r\n"`, the bug is in `splitMultipart`, not in the test.

- [ ] **Step 5: Commit**

```bash
cd ~/code/tinycld/mail
git add server/bodystructure.go server/bodystructure_test.go
git commit -m "feat(mail): pick bodies and files by RFC 8621 parseStructure"
```

---

### Task 4: `parseRFC5322` on the new pipeline

**Files:**
- Modify: `mail/server/imap_rfc5322.go:171-268` (`parseRFC5322`; the old part loop is deleted)
- Create: `mail/server/parsereport.go`
- Modify: `mail/server/smtp_session.go:279-287`, `mail/server/imap_session.go:575-578`, `mail/server/smtp_provider.go:119-122`
- Modify tests: `mail/server/imap_rfc5322_test.go` and any other test that calls `parseRFC5322` (find them with `grep -ln 'parseRFC5322(' server/*_test.go`)

**Interfaces:**
- Consumes: `readTree`, `parseStructure`, `flatten` (Tasks 2–3).
- Produces: `func parseRFC5322(raw []byte) (*storedMessage, parseReport)` and `type parseReport struct{ Warnings []parseWarning; root *mimePart; lists bodyLists; msg *storedMessage }`. Task 8 uses the unexported fields.

- [ ] **Step 1: Write the failing tests**

Change every `msg, err := parseRFC5322(...)` in the tests to `msg, _ := parseRFC5322(...)`, and delete the `if err != nil { t.Fatal(err) }` after it. Then append to `mail/server/imap_rfc5322_test.go`:

```go
// A message with only a subject is valid: it parses to empty bodies and no
// files.
func TestParseRFC5322_SubjectOnly(t *testing.T) {
	t.Parallel()
	msg, report := parseRFC5322([]byte("From: a@example.org\r\nSubject: lunch at noon?\r\n\r\n"))
	if msg.Subject != "lunch at noon?" || msg.TextBody != "" || msg.HTMLBody != "" || len(msg.Attachments) != 0 {
		t.Fatalf("msg = %+v", msg)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("warnings %+v", report.Warnings)
	}
}

// The fault that ended the old part loop: a part in a charset go-message does
// not know dropped every part after it.
func TestParseRFC5322_UnknownCharsetKeepsEveryPart(t *testing.T) {
	t.Parallel()
	raw := "Content-Type: multipart/alternative; boundary=a\r\n\r\n" +
		"--a\r\nContent-Type: text/plain; charset=x-nobody-knows\r\n\r\nCaf\xe9 au lait\r\n" +
		"--a\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Café au lait</p>\r\n--a--\r\n"
	msg, report := parseRFC5322([]byte(raw))
	if msg.TextBody != "Café au lait" || !strings.Contains(msg.HTMLBody, "Café au lait") {
		t.Fatalf("text %q html %q", msg.TextBody, msg.HTMLBody)
	}
	if !hasWarning(report.Warnings, "unknown-charset") {
		t.Fatalf("warnings %+v", report.Warnings)
	}
}

// Review focus 5: a broken inner message does not break the outer one; it is
// kept as an .eml file, not parsed.
func TestParseRFC5322_BrokenInnerMessageIsAFile(t *testing.T) {
	t.Parallel()
	raw := "Content-Type: multipart/mixed; boundary=m\r\n\r\n" +
		"--m\r\nContent-Type: text/plain\r\n\r\nsee attached\r\n" +
		"--m\r\nContent-Type: message/rfc822\r\n\r\n\x00\x01not a message\r\nContent-Type: multipart/mixed; boundary=\r\n" +
		"--m--\r\n"
	msg, _ := parseRFC5322([]byte(raw))
	if msg.TextBody != "see attached" || len(msg.Attachments) != 1 || msg.Attachments[0].Name != "message.eml" {
		t.Fatalf("text %q files %+v", msg.TextBody, msg.Attachments)
	}
}

// Headers are read as before: addresses, ids and the subject.
func TestParseRFC5322_Headers(t *testing.T) {
	t.Parallel()
	raw := "From: Ann <ann@example.org>\r\nTo: bo@example.org\r\nCc: Cy <cy@example.org>\r\n" +
		"Subject: =?utf-8?q?caf=C3=A9?=\r\nMessage-ID: <m1@example.org>\r\nIn-Reply-To: <m0@example.org>\r\n" +
		"References: <a@example.org> <m0@example.org>\r\n\r\nhi\r\n"
	msg, _ := parseRFC5322([]byte(raw))
	if msg.SenderName != "Ann" || msg.SenderEmail != "ann@example.org" || msg.Subject != "café" ||
		msg.MessageID != "<m1@example.org>" || msg.InReplyTo != "<m0@example.org>" ||
		msg.References != "<a@example.org> <m0@example.org>" || len(msg.To) != 1 || len(msg.Cc) != 1 {
		t.Fatalf("msg = %+v", msg)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test -run TestParseRFC5322 .`
Expected: FAIL to compile, because `parseRFC5322` still returns `error` and `parseReport` is undefined.

- [ ] **Step 3: Implement**

Create `mail/server/parsereport.go`:

```go
package mail

// parseReport is what parsing had to salvage. Its caller logs it (Task 8);
// the tree and lists stay so the log can name the message's structure
// without its content.
type parseReport struct {
	Warnings []parseWarning
	root     *mimePart
	lists    bodyLists
	msg      *storedMessage
}
```

In `imap_rfc5322.go`:
1. Add the import `"github.com/emersion/go-message"`.
2. Replace `parseRFC5322` (from its doc comment through the closing brace after `return msg, nil`) with:

```go
// parseRFC5322 turns a raw message into the storedMessage that IMAP APPEND,
// SMTP submission and SMTP inbound store or send. It accepts any input and
// never fails: readTree salvages a part tree, RFC 8621's parseStructure picks
// the bodies and files, and flatten maps them onto storedMessage. The report
// says what had to be salvaged.
func parseRFC5322(raw []byte) (*storedMessage, parseReport) {
	root, warnings := readTree(raw)
	msg := storedHeaderFields(gomail.Header{Header: message.Header{Header: root.Header}})
	lists := parseStructure(root)
	flatten(lists, msg)
	return msg, parseReport{Warnings: warnings, root: root, lists: lists, msg: msg}
}

// storedHeaderFields reads the fields storedMessage keeps from the top-level
// header.
func storedHeaderFields(h gomail.Header) *storedMessage {
	date, _ := h.Date()
	subject, _ := h.Subject()
	messageID, _ := h.MessageID()

	// go-message returns message ids without their angle brackets; these
	// values are stored, matched against stored ids, and relayed as headers
	// by SMTP submission, all of which need the "<id@host>" form.
	msg := &storedMessage{
		MessageID: normalizeMessageID(messageID),
		Subject:   subject,
		Date:      date.UTC().Format(time.RFC3339),
		Headers:   rfc5322Headers(h),
	}

	if inReplyTo, err := h.MsgIDList("In-Reply-To"); err == nil && len(inReplyTo) > 0 {
		msg.InReplyTo = normalizeMessageID(inReplyTo[0])
	}

	if refs, err := h.MsgIDList("References"); err == nil && len(refs) > 0 {
		for i, ref := range refs {
			refs[i] = normalizeMessageID(ref)
		}
		msg.References = strings.Join(refs, " ")
	}

	if from, err := h.AddressList("From"); err == nil && len(from) > 0 {
		msg.SenderName = from[0].Name
		msg.SenderEmail = from[0].Address
	}

	if to, err := h.AddressList("To"); err == nil {
		for _, addr := range to {
			msg.To = append(msg.To, Recipient{Name: addr.Name, Email: addr.Address})
		}
	}

	if cc, err := h.AddressList("Cc"); err == nil {
		for _, addr := range cc {
			msg.Cc = append(msg.Cc, Recipient{Name: addr.Name, Email: addr.Address})
		}
	}
	return msg
}
```

3. Delete the imports this leaves unused (`bytes`, `io`), and run `goimports` or `go vet` to confirm.

In `smtp_session.go`, replace

```go
	msg, err := parseRFC5322(raw)
	if err != nil {
		return s.refuse("data", &smtp.SMTPError{
			Code:         554,
			EnhancedCode: smtp.EnhancedCode{5, 6, 0},
			Message:      "Message could not be parsed",
		}, err)
	}
```

with

```go
	// Any message is accepted: parsing salvages what it can and never fails.
	msg, _ := parseRFC5322(raw)
```

In `imap_session.go`, replace

```go
	msg, err := parseRFC5322(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}
```

with

```go
	msg, _ := parseRFC5322(raw)
```

In `smtp_provider.go`, replace

```go
	stored, err := parseRFC5322(body)
	if err != nil {
		return nil, fmt.Errorf("smtp inbound: %w", err)
	}
```

with

```go
	stored, _ := parseRFC5322(body)
```

and update the doc comment of `ParseInbound`: parsing no longer fails. If `fmt` is now unused in `smtp_provider.go`, remove the import.

- [ ] **Step 4: Run the whole package**

Run: `GOWORK=$HOME/code/tinycld/tinycld/server/go.work go vet . && GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test . 2>&1 | tail -20`
Expected: PASS, including the existing `imap_rfc5322_test.go` cases (split bodies, inline PDF, inline image Content-ID) and the IMAP, SMTP, store and CID tests. A test that expected the old 554 "Message could not be parsed" is now wrong by design: change it to assert the message is accepted.

- [ ] **Step 5: Commit**

```bash
cd ~/code/tinycld/mail
git add server/imap_rfc5322.go server/parsereport.go server/smtp_session.go server/imap_session.go server/smtp_provider.go server/*_test.go
git commit -m "feat(mail): parse every message with readTree and RFC 8621, and never refuse one"
```

---

### Task 5: Vendor the corpus and the fault fixtures

**Files:**
- Create: `mail/server/testdata/corpus/tools/fetch/main.go`
- Create: `mail/server/testdata/corpus/tools/anonymize/main.go`, `main_test.go`
- Create (generated by fetch): `mail/server/testdata/corpus/{stalwart/{rfc,legacy,malformed,regressions,issues,thirdparty},enmime,mimekit,zbateson,cpython,thunderbird}/…`
- Create (by hand): `mail/server/testdata/corpus/faults/*.eml` and `*.flatten.json`, `mail/server/testdata/corpus/clients/.keep`

**Interfaces:**
- Produces: the layout of `testdata/corpus/`:
  - Stalwart messages: each `NAME.eml` has a `NAME.expect.json` beside it, holding `{"text":[…],"html":[…],"attachments":[…]}`. Each entry is `{"type","size","sha256"}` over the part's decoded content, with CRLF changed to LF.
  - Faults and clients: each `NAME.eml` has a `NAME.flatten.json` (format in Task 6).
  - Each source directory has a `SOURCE.md` and its `LICENSE-*` files.

- [ ] **Step 1: Write the anonymizer test**

Create `mail/server/testdata/corpus/tools/anonymize/main_test.go`:

```go
package main

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

const sample = "From: \"Real Person\" <real.person@private.example.net>\r\n" +
	"To: Other <other@private.example.net>\r\n" +
	"Subject: my private subject\r\n" +
	"Message-Id: <ABC-123@private.example.net>\r\n" +
	"Received: from mail.private.example.net\r\n" +
	"Mime-Version: 1.0 (Mac OS X Mail 16.0)\r\n" +
	"Content-Type: multipart/mixed; boundary=\"B\"\r\n\r\n" +
	"--B\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" +
	"Hello Real, caf=C3=A9 tomorrow at 10?\r\n" +
	"--B\r\nContent-Type: application/pdf; name=\"secret plans.pdf\"\r\n" +
	"Content-Disposition: attachment; filename=\"secret plans.pdf\"\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
	strings.Repeat("JVBERi0xLjQKJcfsj6IKNSAwIG9iago8PC9MZW5ndGggNiAwIFIvRmlsdGVyIC9GbGF0ZURlY29kZT4+\r\n", 200) +
	"--B--\r\n"

func TestAnonymize(t *testing.T) {
	out := anonymize([]byte(sample), 256)
	for _, private := range []string{"Real Person", "Hello Real", "private.example.net", "my private subject", "secret plans", "ABC-123", "Received", "tomorrow"} {
		if bytes.Contains(out, []byte(private)) {
			t.Errorf("output still holds %q:\n%s", private, out)
		}
	}
	for _, kept := range []string{"boundary=\"B\"", "--B\r\n", "--B--", "Mac OS X Mail 16.0", "charset=utf-8", "quoted-printable", "=C3=A9", "Content-Disposition: attachment", ".pdf\""} {
		if !bytes.Contains(out, []byte(kept)) {
			t.Errorf("output lost %q:\n%s", kept, out)
		}
	}
	if len(out) > 2000 {
		t.Errorf("attachment not shortened: %d bytes", len(out))
	}
	if !bytes.Contains(out, []byte(`name="file1.pdf"`)) || !bytes.Contains(out, []byte(`filename="file1.pdf"`)) {
		t.Errorf("one file got two names:\n%s", out)
	}
	if n := bytes.Count(out, []byte(base64.StdEncoding.EncodeToString(placeholder(256))[:40])); n != 1 {
		t.Errorf("placeholder written %d times, want once", n)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `cd ~/code/tinycld/mail/server && GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test ./testdata/corpus/tools/anonymize`
Expected: FAIL to compile, `undefined: anonymize`.

- [ ] **Step 3: Write the anonymizer**

Create `mail/server/testdata/corpus/tools/anonymize/main.go`:

```go
// Command anonymize makes a test fixture from a real message. It replaces
// the people, ids, subject, file names and content, and keeps the MIME
// structure byte for byte where it can (boundaries, types, charsets,
// encodings, dispositions), so a fault seen on real mail can be tested
// without checking the mail in. base64 parts are shortened to about -keep
// bytes of placeholder.
//
//	go run ./testdata/corpus/tools/anonymize -in real.eml -out testdata/corpus/faults/x.eml
//
// Read the output before committing it: this tool is a first pass, not a
// guarantee.
package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

func main() {
	in := flag.String("in", "", "real message")
	out := flag.String("out", "", "fixture to write")
	keep := flag.Int("keep", 1024, "bytes of placeholder for each base64 part")
	flag.Parse()
	raw, err := os.ReadFile(*in)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, anonymize(raw, *keep), 0o644); err != nil {
		log.Fatal(err)
	}
}

var (
	emailRe    = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	boundaryRe = regexp.MustCompile(`(?i)boundary\s*=\s*"?([^";\r\n]+)"?`)
	nameRe     = regexp.MustCompile(`(?i)\b((?:file)?name\*?[0-9]*\*?)\s*=\s*("[^"]*"|[^;\s]+)`)
	cteRe      = regexp.MustCompile(`(?i)^content-transfer-encoding:\s*(\S+)`)
	dropRe     = regexp.MustCompile(`(?i)^(received|return-path|delivered-to|dkim-signature|arc-[a-z-]+|authentication-results|x-[a-z0-9-]+|thread-[a-z]+|list-[a-z-]+):`)
	addrRe     = regexp.MustCompile(`(?i)^(from|to|cc|bcc|reply-to|sender):`)
	qpEscape   = regexp.MustCompile(`=[0-9A-Fa-f]{2}|=\r?$`)
)

type anon struct {
	emails map[string]string
	files  map[string]string // original file name → fileN.ext
}

func (a *anon) email(e string) string {
	k := strings.ToLower(e)
	if v, ok := a.emails[k]; ok {
		return v
	}
	v := fmt.Sprintf("person%d@example.com", len(a.emails)+1)
	a.emails[k] = v
	return v
}

func anonymize(raw []byte, keep int) []byte {
	a := &anon{emails: map[string]string{}, files: map[string]string{}}
	eol := "\n"
	if bytes.Contains(raw, []byte("\r\n")) {
		eol = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	boundaries := map[string]bool{}
	var out []string
	inHeader, cte, dropping := true, "", false
	placed := false // the current base64 part's placeholder is written
	for _, line := range lines {
		if inHeader {
			if line == "" {
				inHeader, dropping = false, false
				out = append(out, line)
				continue
			}
			continued := line[0] == ' ' || line[0] == '\t'
			if !continued {
				dropping = dropRe.MatchString(line)
			}
			if dropping {
				continue
			}
			for _, m := range boundaryRe.FindAllStringSubmatch(line, -1) {
				boundaries[m[1]] = true
			}
			if m := cteRe.FindStringSubmatch(line); m != nil {
				cte = strings.ToLower(m[1])
			}
			out = append(out, a.header(line, continued))
			continue
		}
		if isDelimiter(line, boundaries) {
			out = append(out, line)
			inHeader, cte, placed = true, "", false
			continue
		}
		switch cte {
		case "base64":
			if !placed && strings.TrimSpace(line) != "" {
				out = append(out, wrap(base64.StdEncoding.EncodeToString(placeholder(keep)), 76)...)
				placed = true
			}
		case "quoted-printable":
			out = append(out, maskOutside(line, qpEscape))
		default:
			out = append(out, mask(line))
		}
	}
	return []byte(strings.Join(out, eol))
}

func (a *anon) header(line string, continued bool) string {
	lower := strings.ToLower(line)
	switch {
	case !continued && strings.HasPrefix(lower, "subject:"):
		return "Subject: Fixture message"
	case !continued && strings.HasPrefix(lower, "date:"):
		return "Date: Mon, 02 Nov 2026 12:00:00 +0000"
	case !continued && addrRe.MatchString(line):
		name, rest, _ := strings.Cut(line, ":")
		return name + ":" + a.addresses(rest)
	}
	line = emailRe.ReplaceAllStringFunc(line, a.email)
	return nameRe.ReplaceAllStringFunc(line, func(m string) string {
		parts := nameRe.FindStringSubmatch(m)
		original := strings.Trim(parts[2], `"`)
		if _, ok := a.files[original]; !ok {
			ext := ""
			if strings.Contains(original, ".") {
				ext = original[strings.LastIndex(original, "."):]
			}
			// The same file is named the same in Content-Type and
			// Content-Disposition.
			a.files[original] = fmt.Sprintf("file%d%s", len(a.files)+1, ext)
		}
		return fmt.Sprintf(`%s="%s"`, parts[1], a.files[original])
	})
}

// addresses replaces each display name with "Person N" and each address.
func (a *anon) addresses(v string) string {
	var out []string
	for _, addr := range strings.Split(v, ",") {
		e := emailRe.FindString(addr)
		if e == "" {
			continue
		}
		out = append(out, fmt.Sprintf(" Person %s <%s>", strings.TrimSuffix(strings.TrimPrefix(a.email(e), "person"), "@example.com"), a.email(e)))
	}
	return strings.Join(out, ",")
}

func isDelimiter(line string, boundaries map[string]bool) bool {
	t := strings.TrimRight(line, " \t\r")
	if !strings.HasPrefix(t, "--") {
		return false
	}
	t = strings.TrimSuffix(t[2:], "--")
	return boundaries[t]
}

func placeholder(n int) []byte {
	return []byte(strings.Repeat("placeholder ", n/12+1)[:n])
}

func wrap(s string, width int) []string {
	var out []string
	for len(s) > width {
		out = append(out, s[:width])
		s = s[width:]
	}
	return append(out, s)
}

// mask replaces letters with x and digits with 0, keeping length, white
// space and punctuation, so a text part keeps its shape and loses its words.
func mask(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r > 127:
			b.WriteByte('x')
		case r >= '0' && r <= '9':
			b.WriteByte('0')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// maskOutside masks s except where keepRe matches (quoted-printable escapes).
func maskOutside(s string, keepRe *regexp.Regexp) string {
	var b strings.Builder
	last := 0
	for _, loc := range keepRe.FindAllStringIndex(s, -1) {
		b.WriteString(mask(s[last:loc[0]]))
		b.WriteString(s[loc[0]:loc[1]])
		last = loc[1]
	}
	b.WriteString(mask(s[last:]))
	return b.String()
}
```

- [ ] **Step 4: Run the anonymizer test**

Run: `GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test ./testdata/corpus/tools/anonymize -v 2>&1 | tail -5`
Expected: PASS. If a private string survives, fix the anonymizer; the fixture privacy depends on it.

- [ ] **Step 5: Write the fetch tool**

Create `mail/server/testdata/corpus/tools/fetch/main.go`:

```go
// Command fetch refreshes the vendored MIME corpus from its sources at the
// pinned commits, and turns stalwart's expected parses into our compact
// form (NAME.expect.json). Run from mail/server:
//
//	go run ./testdata/corpus/tools/fetch
//
// A refresh is a reviewed diff: change a commit below, run, and read what
// moved. GITHUB_TOKEN, when set, raises the GitHub API rate limit.
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const corpus = "testdata/corpus"

type source struct {
	dest       string
	repo       string
	commit     string
	dir        string
	match      *regexp.Regexp
	ext        string   // rename matched files to this extension; "" keeps the name
	license    []string // license texts in the repo, copied as LICENSE-<base>
	licenseURL string   // a license text fetched from a URL instead
	note       string
	expect     bool // NAME.json beside NAME.eml is stalwart's expected parse
}

const (
	stalwart = "stalwartlabs/mail-parser"
	stalwartCommit = "a56d5f1dc0050a9de123df926081e436334ba9e5"
)

var stalwartLicense = []string{"LICENSES/Apache-2.0.txt", "LICENSES/MIT.txt"}

func stalwartSet(set, note string, extra ...string) source {
	return source{
		dest: "stalwart/" + set, repo: stalwart, commit: stalwartCommit,
		dir: "resources/eml/" + set, match: regexp.MustCompile(`^[^.]+\.(eml|json)$`),
		license: append(append([]string{}, stalwartLicense...), extra...), note: note, expect: true,
	}
}

var sources = []source{
	stalwartSet("rfc", "Examples from the RFCs, from stalwart's mail-parser (Apache-2.0 OR MIT)."),
	stalwartSet("legacy", "The Hunny Software MIME samples (Outlook 2000, Netscape 4.7, Eudora 4.2, Pine), as stalwart redistributes them.", "resources/eml/legacy/COPYING"),
	stalwartSet("malformed", "Malformed messages from stalwart's mail-parser.", "resources/eml/malformed/COPYING"),
	stalwartSet("regressions", "Regression messages from stalwart's mail-parser (see their README for each one's origin).", "resources/eml/regressions/README.md"),
	stalwartSet("issues", "Messages from stalwart mail-parser GitHub issues."),
	stalwartSet("thirdparty", "Dovecot's test messages (MIT), as stalwart redistributes them.", "resources/eml/thirdparty/COPYING"),
	{dest: "enmime", repo: "jhillyerd/enmime", commit: "59a6857f479c1a39d366daf8441f2440637142fc",
		dir: "testdata/mail", match: regexp.MustCompile(`\.raw$`), ext: ".eml", license: []string{"LICENSE"},
		note: "enmime's test messages: bad and unknown charsets, missing types, attachment-only."},
	{dest: "mimekit", repo: "jstedfast/MimeKit", commit: "03097f0474d554284c1fb73692e95aecc1003bec",
		dir: "UnitTests/TestData/messages", match: regexp.MustCompile(`\.(txt|eml)$`), ext: ".eml", license: []string{"LICENSE"},
		note: "MimeKit's test messages: empty multipart, missing subtype, digest, MHTML, delivery status."},
	{dest: "zbateson", repo: "zbateson/mail-mime-parser", commit: "21a8a6ff886f6ab19c065655f84f5a9ac4f86266",
		dir: "tests/_data/emails", match: regexp.MustCompile(`^(github-.*|invalid-.*)\.txt$`), ext: ".eml", license: []string{"LICENSE"},
		note: "zbateson mail-mime-parser's messages from GitHub issues; its Hunny samples are stalwart/legacy."},
	{dest: "cpython", repo: "python/cpython", commit: "017088117964484bd3b8bbf540f08607e4ceb9a1",
		dir: "Lib/test/test_email/data", match: regexp.MustCompile(`^msg_.*\.txt$`), ext: ".eml", license: []string{"LICENSE"},
		note: "CPython's email package test messages."},
	{dest: "thunderbird", repo: "mozilla/releases-comm-central", commit: "a3ea6002cc10342f507de7ae34101c26c6baf4e0",
		dir: "mailnews/test/data", match: regexp.MustCompile(`^mime-torture$`), ext: ".eml",
		licenseURL: "https://www.mozilla.org/media/MPL/2.0/index.txt",
		note: "Thunderbird's MIME torture test: nested multipart and message parts (MPL-2.0)."},
}

func main() {
	for _, s := range sources {
		if err := fetch(s); err != nil {
			log.Fatalf("%s: %v", s.dest, err)
		}
		log.Printf("%s: done", s.dest)
	}
}

func fetch(s source) error {
	dest := filepath.Join(corpus, s.dest)
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	var listing []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}
	if err := getJSON(fmt.Sprintf("https://api.github.com/repos/%s/contents/%s?ref=%s", s.repo, s.dir, s.commit), &listing); err != nil {
		return err
	}
	files := map[string][]byte{}
	for _, f := range listing {
		if f.Type != "file" || !s.match.MatchString(f.Name) {
			continue
		}
		b, err := get(rawURL(s, f.Path))
		if err != nil {
			return err
		}
		files[f.Name] = b
	}
	count := 0
	for name, b := range files {
		if s.expect && strings.HasSuffix(name, ".json") {
			continue
		}
		out := name
		if s.ext != "" {
			out = strings.TrimSuffix(name, path.Ext(name)) + s.ext
		}
		if err := os.WriteFile(filepath.Join(dest, out), b, 0o644); err != nil {
			return err
		}
		count++
		if !s.expect {
			continue
		}
		base := strings.TrimSuffix(name, ".eml")
		js, ok := files[base+".json"]
		if !ok {
			log.Printf("%s/%s has no expected parse; it is checked by LosesNothing only", s.dest, name)
			continue
		}
		exp, err := convert(js, b)
		if err != nil {
			return fmt.Errorf("convert %s: %w", base, err)
		}
		if err := os.WriteFile(filepath.Join(dest, base+".expect.json"), exp, 0o644); err != nil {
			return err
		}
	}
	var licenses []string
	for _, p := range s.license {
		b, err := get(rawURL(s, p))
		if err != nil {
			return err
		}
		name := "LICENSE-" + path.Base(p)
		licenses = append(licenses, name)
		if err := os.WriteFile(filepath.Join(dest, name), b, 0o644); err != nil {
			return err
		}
	}
	if s.licenseURL != "" {
		b, err := get(s.licenseURL)
		if err != nil {
			return err
		}
		licenses = append(licenses, "LICENSE-MPL-2.0.txt")
		if err := os.WriteFile(filepath.Join(dest, "LICENSE-MPL-2.0.txt"), b, 0o644); err != nil {
			return err
		}
	}
	md := fmt.Sprintf("# %s\n\nGenerated by `go run ./testdata/corpus/tools/fetch`; do not edit.\n\n"+
		"%s\n\n- Source: https://github.com/%s/tree/%s/%s\n- Messages: %d\n- License: %s\n",
		s.dest, s.note, s.repo, s.commit, s.dir, count, strings.Join(licenses, ", "))
	return os.WriteFile(filepath.Join(dest, "SOURCE.md"), []byte(md), 0o644)
}

func rawURL(s source, p string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", s.repo, s.commit, p)
}

func get(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" && strings.HasPrefix(url, "https://api.github.com/") {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func getJSON(url string, v any) error {
	b, err := get(url)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// expectPart and expectLists are the compact expected parse; corpus_test.go
// holds the same types.
type expectPart struct {
	Type   string `json:"type"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

type expectLists struct {
	Text        []expectPart `json:"text"`
	HTML        []expectPart `json:"html"`
	Attachments []expectPart `json:"attachments"`
}

type stalwartFile struct {
	Messages []struct {
		TextBody    []int `json:"text_body"`
		HTMLBody    []int `json:"html_body"`
		Attachments []int `json:"attachments"`
	} `json:"messages"`
	Parts []struct {
		ID         int     `json:"id"`
		Kind       string  `json:"kind"`
		Text       *string `json:"text"`
		Binary     *string `json:"binary"`
		OffsetBody int     `json:"offset_body"`
		OffsetEnd  int     `json:"offset_end"`
		Headers    []struct {
			Name  string `json:"name"`
			Value struct {
				ContentType *struct {
					Type    string  `json:"type"`
					Subtype *string `json:"subtype"`
				} `json:"content_type"`
			} `json:"value"`
		} `json:"headers"`
	} `json:"parts"`
}

// convert turns stalwart's expected parse of eml into expectLists, for the
// top-level message: each listed part as its type and the size and SHA-256
// of its decoded content, with CRLF changed to LF.
func convert(js, eml []byte) ([]byte, error) {
	var f stalwartFile
	if err := json.Unmarshal(js, &f); err != nil {
		return nil, err
	}
	if len(f.Messages) == 0 {
		return nil, fmt.Errorf("no message")
	}
	byID := map[int]int{}
	for i, p := range f.Parts {
		byID[p.ID] = i
	}
	part := func(id int) (expectPart, error) {
		i, ok := byID[id]
		if !ok {
			return expectPart{}, fmt.Errorf("no part %d", id)
		}
		p := f.Parts[i]
		typ := map[string]string{"text": "text/plain", "html": "text/html", "message": "message/rfc822"}[p.Kind]
		if typ == "" {
			typ = "application/octet-stream"
		}
		for _, h := range p.Headers {
			if strings.EqualFold(h.Name, "Content-Type") && h.Value.ContentType != nil {
				typ = strings.ToLower(h.Value.ContentType.Type)
				if st := h.Value.ContentType.Subtype; st != nil {
					typ += "/" + strings.ToLower(*st)
				}
			}
		}
		var content []byte
		switch {
		case p.Text != nil:
			content = []byte(*p.Text)
		case p.Binary != nil:
			b, err := base64.StdEncoding.DecodeString(*p.Binary)
			if err != nil {
				return expectPart{}, fmt.Errorf("part %d binary: %w", id, err)
			}
			content = b
		case p.OffsetEnd >= p.OffsetBody && p.OffsetEnd <= len(eml):
			content = eml[p.OffsetBody:p.OffsetEnd]
		}
		content = []byte(strings.ReplaceAll(string(content), "\r\n", "\n"))
		sum := sha256.Sum256(content)
		return expectPart{Type: typ, Size: len(content), SHA256: hex.EncodeToString(sum[:])}, nil
	}
	list := func(ids []int) ([]expectPart, error) {
		out := []expectPart{}
		for _, id := range ids {
			p, err := part(id)
			if err != nil {
				return nil, err
			}
			out = append(out, p)
		}
		return out, nil
	}
	m := f.Messages[0]
	var e expectLists
	var err error
	if e.Text, err = list(m.TextBody); err != nil {
		return nil, err
	}
	if e.HTML, err = list(m.HTMLBody); err != nil {
		return nil, err
	}
	if e.Attachments, err = list(m.Attachments); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(e, "", "  ")
	return append(b, '\n'), err
}
```

- [ ] **Step 6: Fetch the corpus**

Run: `cd ~/code/tinycld/mail/server && GITHUB_TOKEN=$(gh auth token) GOWORK=$HOME/code/tinycld/tinycld/server/go.work go run ./testdata/corpus/tools/fetch`
Expected: one `done` line per source and no error. Then check the result:

```bash
find testdata/corpus -name '*.eml' | wc -l          # about 330
du -sh testdata/corpus                               # about 2.5 MB (the Thunderbird torture test is 1.8 MB of it)
find testdata/corpus/stalwart -name '*.eml' | while read f; do [ -f "${f%.eml}.expect.json" ] || echo "missing ${f%.eml}.expect.json"; done
```

Expected: a "missing" line only for the messages the fetch run logged as having no expected parse.

- [ ] **Step 7: Write the fault fixtures**

Create `mail/server/testdata/corpus/faults/` with these messages and expectations. Each `NAME.flatten.json` follows the format in Task 6.

1. `split-html-inline-pdf.eml`: the value of `appleMailHTMLWithInlinePDF` from `imap_rfc5322_test.go`, written byte for byte (CRLF). `split-html-inline-pdf.flatten.json`:
   ```json
   {"textContains": ["Attach pdf here"], "htmlContains": ["Attach pdf here"], "attachments": [{"name": "doc.pdf", "type": "application/pdf"}]}
   ```
2. `split-text-inline-pdf.eml`: the value of `appleMailTextWithInlinePDF`. `split-text-inline-pdf.flatten.json`:
   ```json
   {"textContains": ["Attach pdf here"], "attachments": [{"name": "doc.pdf", "type": "application/pdf"}]}
   ```
3. `empty-html.eml`:
   ```
   From: person1@example.com
   Subject: Fixture message
   Content-Type: text/html; charset=utf-8

   <html><head><meta charset="utf-8"></head><body></body></html>
   ```
   (CRLF line endings), with `empty-html.flatten.json`: `{"attachments": []}`
4. `subject-only.eml`: the two header lines `From: person1@example.com` and `Subject: Fixture message`, then an empty line and no body (CRLF), with `subject-only.flatten.json`: `{"noBody": true, "attachments": []}`
5. `unknown-charset.eml`, written with printf so the Latin-1 byte stays a raw byte:
   ```bash
   printf 'From: person1@example.com\r\nSubject: Fixture message\r\nContent-Type: multipart/alternative; boundary=a\r\n\r\n--a\r\nContent-Type: text/plain; charset=x-nobody-knows\r\n\r\nCaf\xe9 au lait\r\n--a\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Caf\xc3\xa9 au lait</p>\r\n--a--\r\n' > testdata/corpus/faults/unknown-charset.eml
   ```
   with `unknown-charset.flatten.json`: `{"textContains": ["Café au lait"], "htmlContains": ["Café au lait"], "warnings": ["unknown-charset"], "attachments": []}`
6. `files-no-text.eml`, from the real message, shortened and anonymized:
   ```bash
   go run ./testdata/corpus/tools/anonymize -keep 1024 \
     -in /private/tmp/claude-501/-Users-nas-code-tinycld/bb0f5bda-256f-4519-ad52-a734d932f108/scratchpad/badmail/bad-mail.eml \
     -out testdata/corpus/faults/files-no-text.eml
   ```
   (the scratchpad copy of `~/Downloads/bad-mail.eml`). Then read the output's headers, which are short, and check:
   ```bash
   grep -n -iE 'stitt|argosity|nathan|tinycld\.(com|org)|@' testdata/corpus/faults/files-no-text.eml
   ```
   Expected: only `@example.com` addresses. With `files-no-text.flatten.json` (the anonymizer keeps each original extension, and the two files are an Affinity Photo document and a PDF):
   ```json
   {"noBody": true, "attachments": [{"name": "file1.afphoto", "type": "application/octet-stream"}, {"name": "file2.pdf", "type": "application/pdf"}]}
   ```
   If the fixture's `filename=` parameters carry other names, the fixture is right and this JSON is wrong: copy the names from the fixture.

Create the empty directory for real-client messages: `touch testdata/corpus/clients/.keep`.

- [ ] **Step 8: Commit**

```bash
cd ~/code/tinycld/mail
git add server/testdata/corpus
git commit -m "test(mail): vendor a MIME corpus at pinned commits, and the parser faults as fixtures"
```

---

### Task 6: The corpus tests

**Files:**
- Create: `mail/server/corpus_test.go`
- Create: `mail/server/testdata/corpus/known_deviations.txt`
- Modify: `mail/server/bodystructure.go` (the workarounds the corpus proves, if any)

**Interfaces:**
- Consumes: `readTree`, `leaves`, `parseStructure`, `bodyLists`, `parseRFC5322`, `parseReport` (Tasks 2–4); the corpus (Task 5).
- Produces (used by Task 7): `func corpusPaths(tb testing.TB) []string` and `func checkLosesNothing(t *testing.T, raw []byte)`.

Format of `NAME.flatten.json`:

```json
{
  "textContains": ["…"],
  "htmlContains": ["…"],
  "noBody": false,
  "warnings": ["unknown-charset"],
  "attachments": [{"name": "doc.pdf", "type": "application/pdf"}]
}
```

Every field is optional except `attachments`, which is the exact list in order. `noBody` means both bodies are empty. `warnings` is a subset of the warning kinds expected.

`known_deviations.txt` has one line per stalwart message whose expected lists we knowingly do not match: `<path under testdata/corpus><TAB><reason>`. Lines starting with `#` are comments.

- [ ] **Step 1: Write the tests**

Create `mail/server/corpus_test.go`:

```go
package mail

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The corpus gates the parser: every message must lose nothing, stalwart's
// messages must match their expected parse (or be a listed deviation), and
// the fault and client messages must flatten as their .flatten.json says.
// See testdata/corpus/*/SOURCE.md for where each message came from.

const corpusRoot = "testdata/corpus"

func corpusPaths(tb testing.TB) []string {
	tb.Helper()
	var paths []string
	err := filepath.WalkDir(corpusRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "tools" {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(p, ".eml") {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	sort.Strings(paths)
	if len(paths) < 300 {
		tb.Fatalf("found %d corpus messages, want the vendored ~330; run the fetch tool", len(paths))
	}
	return paths
}

func corpusName(p string) string { return strings.TrimPrefix(p, corpusRoot+"/") }

// checkLosesNothing asserts the salvage guarantees on one message.
func checkLosesNothing(t *testing.T, raw []byte) {
	t.Helper()
	root, _ := readTree(raw)
	l := parseStructure(root)
	listed := map[*mimePart]bool{}
	for _, list := range [][]*mimePart{l.text, l.html, l.attachments} {
		for _, p := range list {
			listed[p] = true
		}
	}
	textLeaf := false
	for _, leaf := range leaves(root) {
		if len(leaf.Body) > 0 && !listed[leaf] {
			t.Errorf("part %d (%s, %d bytes) is in no list", leaf.Index, leaf.Type, len(leaf.Body))
		}
		if leaf.RawSize > 0 && len(leaf.Body) == 0 {
			t.Errorf("part %d (%s) had %d bytes and kept none", leaf.Index, leaf.Type, leaf.RawSize)
		}
		if strings.HasPrefix(leaf.Type, "text/") && leaf.Disposition != "attachment" && len(bytes.TrimSpace(leaf.Body)) > 0 {
			textLeaf = true
		}
	}
	msg, _ := parseRFC5322(raw)
	if textLeaf && strings.TrimSpace(msg.TextBody) == "" && strings.TrimSpace(msg.HTMLBody) == "" {
		t.Errorf("the message has text, but both bodies are empty")
	}
	again, _ := parseRFC5322(raw)
	if !reflect.DeepEqual(msg, again) {
		t.Errorf("parsing twice gave different results")
	}
}

func TestCorpus_LosesNothing(t *testing.T) {
	t.Parallel()
	for _, p := range corpusPaths(t) {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(corpusName(p), func(t *testing.T) {
			t.Parallel()
			checkLosesNothing(t, raw)
		})
	}
}

type expectPart struct {
	Type   string `json:"type"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

type expectLists struct {
	Text        []expectPart `json:"text"`
	HTML        []expectPart `json:"html"`
	Attachments []expectPart `json:"attachments"`
}

func toExpect(ps []*mimePart) []expectPart {
	out := []expectPart{}
	for _, p := range ps {
		content := []byte(strings.ReplaceAll(string(p.Body), "\r\n", "\n"))
		sum := sha256.Sum256(content)
		out = append(out, expectPart{Type: p.Type, Size: len(content), SHA256: hex.EncodeToString(sum[:])})
	}
	return out
}

func readDeviations(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open(filepath.Join(corpusRoot, "known_deviations.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, reason, ok := strings.Cut(line, "\t")
		if !ok || strings.TrimSpace(reason) == "" {
			t.Fatalf("known_deviations.txt: %q has no reason", line)
		}
		out[name] = reason
	}
	return out
}

func TestCorpus_MatchesExpected(t *testing.T) {
	t.Parallel()
	deviations := readDeviations(t)
	seen := map[string]bool{}
	for _, p := range corpusPaths(t) {
		expPath := strings.TrimSuffix(p, ".eml") + ".expect.json"
		b, err := os.ReadFile(expPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var want expectLists
		if err := json.Unmarshal(b, &want); err != nil {
			t.Fatalf("%s: %v", expPath, err)
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		name := corpusName(p)
		seen[name] = true
		t.Run(name, func(t *testing.T) {
			root, _ := readTree(raw)
			l := parseStructure(root)
			got := expectLists{Text: toExpect(l.text), HTML: toExpect(l.html), Attachments: toExpect(l.attachments)}
			match := reflect.DeepEqual(got, want)
			reason, listed := deviations[name]
			switch {
			case !match && !listed:
				g, _ := json.MarshalIndent(got, "", "  ")
				w, _ := json.MarshalIndent(want, "", "  ")
				t.Errorf("lists differ from stalwart's\ngot:  %s\nwant: %s", g, w)
			case match && listed:
				t.Errorf("listed as a deviation (%s) but now matches: remove it from known_deviations.txt", reason)
			}
		})
	}
	for name := range deviations {
		if !seen[name] {
			t.Errorf("known_deviations.txt lists %s, which has no expected parse", name)
		}
	}
}

type flattenExpect struct {
	TextContains []string `json:"textContains"`
	HTMLContains []string `json:"htmlContains"`
	NoBody       bool     `json:"noBody"`
	Warnings     []string `json:"warnings"`
	Attachments  []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"attachments"`
}

func TestCorpus_Flatten(t *testing.T) {
	t.Parallel()
	for _, p := range corpusPaths(t) {
		b, err := os.ReadFile(strings.TrimSuffix(p, ".eml") + ".flatten.json")
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var want flattenExpect
		if err := json.Unmarshal(b, &want); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(corpusName(p), func(t *testing.T) {
			msg, report := parseRFC5322(raw)
			for _, s := range want.TextContains {
				if !strings.Contains(msg.TextBody, s) {
					t.Errorf("text %q lacks %q", msg.TextBody, s)
				}
			}
			for _, s := range want.HTMLContains {
				if !strings.Contains(msg.HTMLBody, s) {
					t.Errorf("html %q lacks %q", msg.HTMLBody, s)
				}
			}
			empty := strings.TrimSpace(msg.TextBody) == "" && strings.TrimSpace(msg.HTMLBody) == ""
			if empty != want.NoBody {
				t.Errorf("both bodies empty = %v, want %v", empty, want.NoBody)
			}
			for _, kind := range want.Warnings {
				if !hasWarning(report.Warnings, kind) {
					t.Errorf("no %s warning in %+v", kind, report.Warnings)
				}
			}
			if len(msg.Attachments) != len(want.Attachments) {
				t.Fatalf("attachments = %d, want %d: %+v", len(msg.Attachments), len(want.Attachments), msg.Attachments)
			}
			for i, a := range want.Attachments {
				if got := msg.Attachments[i]; got.Name != a.Name || got.ContentType != a.Type {
					t.Errorf("attachment %d = %s (%s), want %s (%s)", i, got.Name, got.ContentType, a.Name, a.Type)
				}
			}
		})
	}
}
```

Create `mail/server/testdata/corpus/known_deviations.txt`:

```
# Stalwart messages whose expected lists we knowingly do not match.
# One per line: <path under testdata/corpus><TAB><reason>.
# TestCorpus_MatchesExpected fails on a listed message that now matches, so
# the list can only shrink.
```

- [ ] **Step 2: Run the tests and collect the failures**

Run:
```bash
cd ~/code/tinycld/mail/server
GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test -run 'TestCorpus' . > /tmp/corpus.log 2>&1; grep -E '^\s+--- FAIL' /tmp/corpus.log | sed 's/(.*//' | sort | uniq -c | sort -rn | head -40
```

Expected:
- `TestCorpus_Flatten` passes for every fault fixture. If one fails, the bug is in Tasks 2–4: fix it there.
- `TestCorpus_LosesNothing` may fail on some messages. Every such failure is a bug in `readTree` or `parseStructure`: fix the code. **No deviation list exists for this test.**
- `TestCorpus_MatchesExpected` will show mismatches.

- [ ] **Step 3: Settle each `MatchesExpected` mismatch**

For each mismatching message, read the `.eml` and the two lists in the failure output, then decide:

- **Workaround.** Stalwart's lists are what a person reading the message expects to see, and ours differ because the RFC's starting-point algorithm handles that shape badly (stalwart's issue #67 fixes are the likely ones: an alternative child with disposition attachment, a named HTML part inside an alternative, an unterminated alternative). Then:
  - add a small rule to `structure` in `bodystructure.go`;
  - add a line to the "Workarounds:" list in the file's header comment, naming the corpus message (`- <rule>: stalwart/regressions/alternative-named-html.eml`);
  - add a case to `TestParseStructure_RFC8621` that pins the rule.
- **Deviation.** The difference comes from a rule of ours that the spec requires and stalwart does not follow. Examples:
  - We never drop a part (`backfill`), where stalwart's `other_parts` drops it.
  - Lenient decoding keeps bytes that stalwart discards.

  Add the message to `known_deviations.txt` with that reason. A difference in decoded content (charset, base64, line breaks) is never a deviation: it is a decoder bug in `mimetree.go`. Fix it.

Rerun the command from Step 2 until `TestCorpus` passes fully.

- [ ] **Step 4: Run the whole package**

Run: `GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test -race . 2>&1 | tail -5`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd ~/code/tinycld/mail
git add server/corpus_test.go server/bodystructure.go server/bodystructure_test.go server/mimetree.go server/testdata/corpus/known_deviations.txt
git commit -m "test(mail): gate the parser on the corpus: lose nothing, match stalwart, flatten the faults"
```

---

### Task 7: Fuzz the parser

**Files:**
- Create: `mail/server/corpus_fuzz_test.go`

**Interfaces:**
- Consumes: `corpusPaths`, `checkLosesNothing` (Task 6).

- [ ] **Step 1: Write the fuzz test**

```go
package mail

import (
	"os"
	"testing"
	"time"
)

// FuzzParseRFC5322 starts from every corpus message. Whatever the input, the
// parser must not panic, must keep the salvage guarantees, and must finish
// quickly (a slow input is a quadratic path an attacker can use). In CI it
// runs over the seeds; run it longer locally with
// go test -fuzz FuzzParseRFC5322 -fuzztime 5m .
func FuzzParseRFC5322(f *testing.F) {
	for _, p := range corpusPaths(f) {
		raw, err := os.ReadFile(p)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		done := make(chan struct{})
		go func() {
			defer close(done)
			checkLosesNothing(t, raw)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("parsing %d bytes took over a second", len(raw))
		}
	})
}
```

- [ ] **Step 2: Run over the seeds, then fuzz for a while**

Run: `GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test -run FuzzParseRFC5322 . && GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test -run '^$' -fuzz FuzzParseRFC5322 -fuzztime 2m .`
Expected: PASS, and the fuzzing run ends with no failure. A failing input is saved under `testdata/fuzz/FuzzParseRFC5322/`. Fix the parser for it, and keep the saved input: it becomes a seed.

- [ ] **Step 3: Commit**

```bash
cd ~/code/tinycld/mail
git add server/corpus_fuzz_test.go
# if Step 2 saved a failing input you fixed, keep it as a seed:
git add server/testdata/fuzz/FuzzParseRFC5322 2>/dev/null || true
git commit -m "test(mail): fuzz the parser from the corpus"
```

---

### Task 8: Log what had to be salvaged

**Files:**
- Modify: `mail/server/parsereport.go`
- Modify: `mail/server/smtp_session.go`, `mail/server/imap_session.go`, `mail/server/smtp_provider.go` (the parse calls from Task 4)
- Test: `mail/server/parsereport_test.go`

**Interfaces:**
- Consumes: `parseReport` (Task 4).
- Produces: `func (r parseReport) needsLog() bool`, `func (r parseReport) structure() string`, `func (r parseReport) log(route, user string)`.

- [ ] **Step 1: Write the failing tests**

```go
package mail

import (
	"strings"
	"testing"
)

func TestParseReport_Structure(t *testing.T) {
	t.Parallel()
	_, r := parseRFC5322(crlf("Content-Type: multipart/mixed; boundary=m\n\n" +
		"--m\nContent-Type: text/html; charset=x-nobody-knows\n\n<p>caf\xe9</p>\n" +
		"--m\nContent-Type: application/pdf; name=\"private name.pdf\"\nContent-Disposition: inline\n\nPDF\n--m--\n"))
	got := r.structure()
	want := `0 multipart/mixed; 1 text/html charset=x-nobody-knows 12B →text+html; 2 application/pdf inline 3B →file`
	if got != want {
		t.Fatalf("structure =\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, "private") || strings.Contains(got, "caf") {
		t.Fatalf("structure leaks content: %s", got)
	}
	if !r.needsLog() {
		t.Fatal("a salvaged charset does not log")
	}
}

func TestParseReport_NeedsLog(t *testing.T) {
	t.Parallel()
	if _, r := parseRFC5322(crlf("Content-Type: text/plain\n\nhi\n")); r.needsLog() {
		t.Fatal("a clean message logs")
	}
	if _, r := parseRFC5322(crlf("Subject: only a subject\n\n")); r.needsLog() {
		t.Fatal("a subject-only message logs; it is valid mail")
	}
}
```

`12B` is the decoded UTF-8 body `<p>café</p>` (`é` is two bytes). The line break before the delimiter is not part of the body (Task 2).

- [ ] **Step 2: Run them to verify they fail**

Run: `GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test -run TestParseReport .`
Expected: FAIL to compile, `r.structure undefined`.

- [ ] **Step 3: Implement**

Append to `mail/server/parsereport.go`:

```go
import (
	"fmt"
	"strings"

	"tinycld.org/core/logging"
)

var parseLog = logging.ForPackage("mail")

// needsLog is true when parsing had to salvage something, or when the
// message has text that ended up in neither body. A subject-only message is
// valid mail and logs nothing.
func (r parseReport) needsLog() bool {
	if len(r.Warnings) > 0 {
		return true
	}
	if r.msg == nil || r.root == nil {
		return false
	}
	if strings.TrimSpace(r.msg.TextBody) != "" || strings.TrimSpace(r.msg.HTMLBody) != "" {
		return false
	}
	for _, leaf := range leaves(r.root) {
		if strings.HasPrefix(leaf.Type, "text/") && leaf.Disposition != "attachment" && strings.TrimSpace(string(leaf.Body)) != "" {
			return true
		}
	}
	return false
}

// structure names each part's index, type, charset, disposition, size and
// list, never its name or content, so a bad message can be diagnosed from
// the log without the log holding the mail.
func (r parseReport) structure() string {
	in := map[*mimePart][]string{}
	for _, e := range []struct {
		list []*mimePart
		name string
	}{{r.lists.text, "text"}, {r.lists.html, "html"}, {r.lists.attachments, "file"}} {
		for _, p := range e.list {
			in[p] = append(in[p], e.name)
		}
	}
	var parts []string
	var walk func(p *mimePart)
	walk = func(p *mimePart) {
		s := fmt.Sprintf("%d %s", p.Index, p.Type)
		if cs := p.Params["charset"]; cs != "" {
			s += " charset=" + cs
		}
		if p.Disposition != "" {
			s += " " + p.Disposition
		}
		if !p.isMultipart() {
			s += fmt.Sprintf(" %dB", len(p.Body))
			if lists := in[p]; len(lists) > 0 {
				s += " →" + strings.Join(lists, "+")
			}
		}
		parts = append(parts, s)
		for _, c := range p.Children {
			walk(c)
		}
	}
	if r.root != nil {
		walk(r.root)
	}
	return strings.Join(parts, "; ")
}

func (r parseReport) warningText() string {
	var ws []string
	for _, w := range r.Warnings {
		ws = append(ws, fmt.Sprintf("%d %s: %s", w.Part, w.Kind, w.Detail))
	}
	return strings.Join(ws, "; ")
}

// log leaves one warning line when needsLog is true. route says which path
// parsed the message; user is the account it belongs to, or "".
func (r parseReport) log(route, user string) {
	if !r.needsLog() {
		return
	}
	parseLog.Warn("message parsed with problems", "route", route, "user", user,
		"structure", r.structure(), "warnings", r.warningText())
}
```

Merge the new imports into the file's single `import` block.

`warningText` writes the warning details, which Task 2 keeps free of header values.

Wire the callers:
- `smtp_session.go`: `msg, report := parseRFC5322(raw)`, then `report.log("smtp-submission", s.user.Id)` (`s.user` is set by AUTH before DATA).
- `imap_session.go`: `msg, report := parseRFC5322(raw)`, then `report.log("imap-append", s.user.Id)`.
- `smtp_provider.go`: `stored, report := parseRFC5322(body)`, then `report.log("smtp-inbound", "")`.

- [ ] **Step 4: Run the tests**

Run: `GOWORK=$HOME/code/tinycld/tinycld/server/go.work go vet . && GOWORK=$HOME/code/tinycld/tinycld/server/go.work go test -race . 2>&1 | tail -5`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd ~/code/tinycld/mail
git add server/parsereport.go server/parsereport_test.go server/smtp_session.go server/imap_session.go server/smtp_provider.go
git commit -m "feat(mail): log a salvaged message's structure, never its content"
```

---

### Task 9: Docs and the spec's rulings

**Files:**
- Modify: `mail/README.md` (the server layout lines: `imap_rfc5322.go` and the new files)
- Modify: `mail/docs/superpowers/specs/2026-11-04-mime-parser-rfc8621-design.md`

- [ ] **Step 1: README**

In the server file list of `mail/README.md`:
- Change the `imap_rfc5322.go` line so it says the file builds messages for FETCH and holds `parseRFC5322`, which IMAP APPEND, SMTP submission and SMTP inbound use.
- Add these lines:
  - `mimetree.go` — `readTree`: salvages a MIME tree from any input (its own header parser, boundary splitter and lenient decoders).
  - `bodystructure.go` — RFC 8621 parseStructure and `flatten` onto `storedMessage`.
  - `parsereport.go` — what parsing salvaged, and the structure-only log line.
  - `testdata/corpus/` — the vendored corpus that gates the parser; refresh it with `go run ./testdata/corpus/tools/fetch`.

- [ ] **Step 2: Record the plan's rulings in the spec**

Update the spec so it matches what was built:
1. `parseRFC5322` returns `(*storedMessage, parseReport)`, where `parseReport` carries the warnings and, unexported, the tree and lists for the log.
2. Fault and client expectations are `NAME.flatten.json`, not `NAME.expect.json` (that name is the stalwart lists).
3. Children of `multipart/digest` default to `message/rfc822` (RFC 2046 §5.1.5).
4. `parseStructure` adds every leaf the algorithm put in no list to attachments (`backfill`), following RFC 8621's definition of attachments.
5. Warning details name field names, lengths and charsets, never header values.
6. The Thunderbird torture test (1.8 MB) is vendored unchanged, so the corpus is about 2.5 MB, not 0.7 MB.

- [ ] **Step 3: Run the whole package check, then commit**

Run: `cd ~/code/tinycld/mail && pnpm exec tinycld-pkg check 2>&1 | tail -15`
Expected: PASS (biome, tsc, vitest and the Go tests).

```bash
cd ~/code/tinycld/mail
git add README.md docs/superpowers/specs/2026-11-04-mime-parser-rfc8621-design.md
git commit -m "docs(mail): the RFC 8621 parser and its corpus"
```

---

## After the plan

The mail release (patch), its deploy to tinycld.com (`pnpm run hosting -- expedite mail@<version>`), and the real-client captures are not tasks here. Release after the branch is reviewed and merged.
