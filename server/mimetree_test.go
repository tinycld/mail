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
		{"declared UTF-8, invalid", "text/plain", "utf-8", []byte("caf\xff"), "caf�", "invalid-utf8"},
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
		name    string
		raw     []byte
		warning string
		check   func(t *testing.T, root *mimePart)
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
