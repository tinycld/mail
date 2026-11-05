package mail

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"
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

	h, body, problem := parseHeaderBlock(crlf("Subject: one\n two\nFrom: a@example.org\n\nbody\n"), maxHeaderFields)
	if problem != "" || h.Get("Subject") != "one two" || h.Get("From") != "a@example.org" || string(body) != "body\r\n" {
		t.Fatalf("got subject %q from %q body %q problem %q", h.Get("Subject"), h.Get("From"), body, problem)
	}

	// A broken line between fields is skipped; the fields after it stay.
	h, body, problem = parseHeaderBlock(crlf("Subject: s\nthis is not a field\nContent-Type: text/html\n\n<p>x</p>"), maxHeaderFields)
	if h.Get("Content-Type") != "text/html" || string(body) != "<p>x</p>" || problem == "" {
		t.Fatalf("skip: ct %q body %q problem %q", h.Get("Content-Type"), body, problem)
	}

	// A body line with a colon right after the fields ends the header: it is
	// body, and no byte is dropped. This holds when a blank line follows it.
	for _, tc := range []struct{ in, field, body string }{
		{"Content-Type: text/plain\nHi Bob: lunch at noon?\n\nthanks", "Content-Type", "Hi Bob: lunch at noon?\r\n\r\nthanks"},
		{"Subject: s\nDear team: see below\n\nthe report", "Subject", "Dear team: see below\r\n\r\nthe report"},
		{"Subject: s\nHi Bob: see this\nthanks\n\nmore\n", "Subject", "Hi Bob: see this\r\nthanks\r\n\r\nmore\r\n"},
		{"From: good@x.com\nFr om: evil@x.com\n\nbody\n", "From", "Fr om: evil@x.com\r\n\r\nbody\r\n"},
	} {
		h, body, problem = parseHeaderBlock(crlf(tc.in), maxHeaderFields)
		if h.Len() != 1 || h.Get(tc.field) == "" || string(body) != tc.body || problem == "" {
			t.Fatalf("colon line in %q: %d fields, body %q, problem %q", tc.in, h.Len(), body, problem)
		}
	}

	// A body line over an indented line right after the fields ends the
	// header: all three body lines are kept.
	h, body, problem = parseHeaderBlock(crlf("Content-Type: text/plain\nDear Bob,\n   indented\n\nthanks"), maxHeaderFields)
	if h.Get("Content-Type") != "text/plain" || h.Len() != 1 || string(body) != "Dear Bob,\r\n   indented\r\n\r\nthanks" || problem == "" {
		t.Fatalf("indented body line: %d fields, body %q, problem %q", h.Len(), body, problem)
	}

	// A skipped line takes no continuation: the field after it starts anew.
	h, body, _ = parseHeaderBlock(crlf("From: a@x.com\nnot a field\nTo: b@x.com\n c@x.com\n\nbody"), maxHeaderFields)
	if h.Get("From") != "a@x.com" || h.Get("To") != "b@x.com c@x.com" || h.Len() != 2 || string(body) != "body" {
		t.Fatalf("skip then field: from %q to %q, %d fields, body %q", h.Get("From"), h.Get("To"), h.Len(), body)
	}

	// With no readable field, the header block cannot be read: all is body.
	in0 := crlf("Sub ject: smuggled\nSub\tject: tab\n\nbody\n")
	if h, body, _ = parseHeaderBlock(in0, maxHeaderFields); h.Len() != 0 || !bytes.Equal(body, in0) {
		t.Fatalf("unreadable: %d fields, body %q", h.Len(), body)
	}

	// A broken line with no field after it ends the header; nothing is lost.
	h, body, problem = parseHeaderBlock(crlf("Subject: s\nnot a field\nmore text\n"), maxHeaderFields)
	if h.Get("Subject") != "s" || string(body) != "not a field\r\nmore text\r\n" || problem == "" {
		t.Fatalf("end: body %q problem %q", body, problem)
	}

	// Input that does not start with a field has no header at all.
	in := []byte("\x00\x01binary garbage\nmore")
	h, body, _ = parseHeaderBlock(in, maxHeaderFields)
	if h.Len() != 0 || !bytes.Equal(body, in) {
		t.Fatalf("no header: %d fields, body %q", h.Len(), body)
	}

	// The first of two fields with one name is what Get returns, and the
	// fields iterate in message order.
	h, _, _ = parseHeaderBlock(crlf("Received: first\nReceived: second\n\n"), maxHeaderFields)
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

// A body of only "=" decodes to nothing; the raw body is kept so the part
// does not lose its bytes (found by fuzzing).
func TestReadTree_QuotedPrintableWithNothingDecodableIsKeptRaw(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"=", "= \t"} {
		raw := "Content-Type: text/plain\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" + body
		root, warnings := readTree([]byte(raw))
		if string(root.Body) != body {
			t.Errorf("body %q kept as %q", body, root.Body)
		}
		found := false
		for _, w := range warnings {
			found = found || w.Kind == "bad-encoding"
		}
		if !found {
			t.Errorf("body %q: no bad-encoding warning in %v", body, warnings)
		}
	}
}

// The delimiter takes the part's last line break, so a final "=" there is a
// real soft break and must not show.
func TestReadTree_QuotedPrintableSoftBreakBeforeDelimiter(t *testing.T) {
	t.Parallel()
	raw := "Content-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\nContent-Type: text/plain\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\nsoft=\r\n--b--\r\n"
	root, warnings := readTree([]byte(raw))
	if got := string(root.Children[0].Body); got != "soft" {
		t.Errorf("got %q", got)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings: %v", warnings)
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
		{"html stale meta on UTF-8", "text/html", "", []byte(`<meta charset="iso-8859-1"><p>café`), `<meta charset="iso-8859-1"><p>café`, ""},
		{"html unknown label uses meta", "text/html", "x-nobody-knows", []byte(`<meta charset="iso-8859-1"><p>caf` + "\xe9"), `<meta charset="iso-8859-1"><p>café`, "unknown-charset"},
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

func TestParseHeaderBlock_ManyContinuationLines(t *testing.T) {
	t.Parallel()
	// 200k folded lines: string concatenation per line would copy GBs.
	in := "Subject: a\r\n" + strings.Repeat(" a\r\n", 200_000) + "\r\nbody"
	h, body, problem := parseHeaderBlock([]byte(in), maxHeaderFields)
	if got := len(h.Get("Subject")); got != 1+2*200_000 || string(body) != "body" || problem != "" {
		t.Fatalf("subject length %d body %q problem %q", got, body, problem)
	}
}

func TestParseHeaderBlock_ManyBrokenLines(t *testing.T) {
	t.Parallel()
	// As many skipped lines as the field cap allows, each before a field:
	// each is decided by one line of lookahead, so the header is read in one
	// pass.
	n := maxHeaderFields - 1
	in := "Subject: a\r\n" + strings.Repeat("x\r\nX-A: b\r\n", n) + "\r\nbody"
	start := time.Now()
	h, body, _ := parseHeaderBlock([]byte(in), maxHeaderFields)
	if h.Get("Subject") != "a" || len(h.Values("X-A")) != n || string(body) != "body" {
		t.Fatalf("subject %q, %d X-A fields, body of %d bytes", h.Get("Subject"), len(h.Values("X-A")), len(body))
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestParseHeaderBlock_MboxFromLine(t *testing.T) {
	t.Parallel()
	h, body, problem := parseHeaderBlock(crlf("From sender@example.org Tue Nov  4 10:00:00 2026\nSubject: s\nContent-Type: text/html\n\n<p>x</p>"), maxHeaderFields)
	if h.Get("Subject") != "s" || h.Get("Content-Type") != "text/html" || string(body) != "<p>x</p>" || problem == "" {
		t.Fatalf("subject %q ct %q body %q problem %q", h.Get("Subject"), h.Get("Content-Type"), body, problem)
	}
	if strings.Contains(problem, "sender@example.org") {
		t.Fatalf("problem quotes the line: %q", problem)
	}
}

func TestReadTree_LenientContentType(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, header, typ, param, value string
	}{
		{"no semicolon before charset", "Content-Type: text/html charset=utf-8", "text/html", "charset", "utf-8"},
		{"no semicolon before boundary", "Content-Type: multipart/mixed boundary=x", "multipart/mixed", "boundary", "x"},
		{"no semicolon before name", "Content-Type: application/pdf name=x.pdf", "application/pdf", "name", "x.pdf"},
		{"no usable type", "Content-Type: ;;; charset=utf-8", "text/plain", "charset", "utf-8"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, warnings := readTree(crlf(tc.header + "\n\n--x\n\nbody\n--x--\n"))
			if root.Type != tc.typ || root.Params[tc.param] != tc.value {
				t.Fatalf("type %q params %v", root.Type, root.Params)
			}
			if !hasWarning(warnings, "bad-content-type") {
				t.Fatalf("warnings %+v", warnings)
			}
			for _, w := range warnings {
				if w.Kind == "bad-content-type" && strings.Contains(w.Detail, "=") {
					t.Fatalf("detail quotes the header: %q", w.Detail)
				}
			}
		})
	}
}

func TestDecodeUTF7(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in, want   string
		wellFormed bool
	}{
		{"+AKM.", "£.", true},
		{"x +AKM y", "x £ y", true},
		{"+AKM, then", "£, then", true},
		{"1 +- 1", "1 + 1", true},
		{"+AKM-x", "£x", true},
		{"+AKM--x", "£-x", true},
		{"Fr+APY-sche", "Frösche", true},
		{"+2D3eAA-", "\U0001F600", true},
		{"lone +2D0- surrogate", "lone \uFFFD surrogate", true},
		{"no runs at all, only ", "no runs at all, only ", true},
		// A plus that opens no code unit is a literal a sender did not escape.
		{"1 + 1", "1 + 1", false},
		{"C++ code", "C++ code", false},
		{"x+y", "x+y", false},
		{"end+", "end+", false},
		// 6 or more bits left over are a cut-off code unit.
		{"+AKMA-", "£\uFFFD", false},
	} {
		got, wellFormed := decodeUTF7([]byte(tc.in))
		if string(got) != tc.want || wellFormed != tc.wellFormed {
			t.Errorf("decodeUTF7(%q) = %q, %v; want %q, %v", tc.in, got, wellFormed, tc.want, tc.wellFormed)
		}
	}

	root, warnings := readTree(crlf("Content-Type: text/plain; charset=utf-7\n\nDie Fr+APY-sche\n"))
	if string(root.Body) != "Die Frösche\r\n" || len(warnings) != 0 {
		t.Fatalf("body %q warnings %+v", root.Body, warnings)
	}
	root, warnings = readTree(crlf("Content-Type: text/plain; charset=utf-7\n\nC++ is 1 + 1\n"))
	if string(root.Body) != "C++ is 1 + 1\r\n" || !hasWarning(warnings, "bad-encoding") {
		t.Fatalf("ill-formed: body %q warnings %+v", root.Body, warnings)
	}
	if strings.Contains(warnings[0].Detail, "C++") {
		t.Fatalf("warning quotes the text: %q", warnings[0].Detail)
	}
}

func TestToValidUTF8_MaximalSubparts(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"ok \xc3\xa9":          "ok \xc3\xa9",
		"\x80\x81\x82j":        "\uFFFD\uFFFD\uFFFDj",
		"\xe2\x82x":            "\uFFFDx",
		"\xf0\x9f\x98":         "\uFFFD",
		"\xed\xa0\x80":         "\uFFFD\uFFFD\uFFFD",
		"\xc0\xaf":             "\uFFFD\uFFFD",
		"a\xef\xbf\xbdb":       "a\uFFFDb",
		"\xf4\x90\x80\x80":     "\uFFFD\uFFFD\uFFFD\uFFFD",
		"\xe0\xa0\x80\xe0\x80": "\xe0\xa0\x80\uFFFD\uFFFD",
	} {
		if got := string(toValidUTF8([]byte(in))); got != want {
			t.Errorf("toValidUTF8(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReadTree_FinalLoneCRIsNotContent(t *testing.T) {
	t.Parallel()
	root, _ := readTree([]byte("Subject: s\r\n\r\nend\r\n\r"))
	if string(root.Body) != "end\r\n" {
		t.Fatalf("body %q", root.Body)
	}
}

func TestReadTree_DispositionWithoutType(t *testing.T) {
	t.Parallel()
	root, _ := readTree(crlf("Content-Type: multipart/mixed; boundary=b\n\n" +
		"--b\nContent-Type: image/png\nContent-Disposition: ;\n filename=\"a.png\"\n\nPNG\n" +
		"--b\nContent-Type: application/pdf\nContent-Disposition: x-unknown; filename=b.pdf\n\nPDF\n--b--\n"))
	if p := root.Children[0]; p.Disposition != "" || p.Name != "a.png" {
		t.Errorf("no type: disposition %q name %q", p.Disposition, p.Name)
	}
	if p := root.Children[1]; p.Disposition != "attachment" || p.Name != "b.pdf" {
		t.Errorf("unknown type: disposition %q name %q", p.Disposition, p.Name)
	}
}

func TestParseContentType_NoSubtype(t *testing.T) {
	t.Parallel()
	typ, params, ok := parseContentType("multipart;boundary=b5", "text/plain")
	if typ != "text/plain" || params["boundary"] != "b5" || ok {
		t.Fatalf("type %q params %v ok %v", typ, params, ok)
	}
}

// RFC 2046 drops the epilogue; a dropped epilogue that is not blank gets a
// warning that names only its length.
func TestReadTree_EpilogueIsDroppedWithAWarning(t *testing.T) {
	t.Parallel()
	root, warnings := readTree(crlf("Content-Type: multipart/mixed; boundary=b\n\n--b\n\none\n--b--\nsecret epilogue\n"))
	if len(root.Children) != 1 || string(root.Children[0].Body) != "one" {
		t.Fatalf("children %+v", root.Children)
	}
	w := warningOfKind(t, warnings, "bad-multipart", "epilogue of 15 bytes")
	if strings.Contains(w.Detail, "secret") {
		t.Fatalf("warning quotes the epilogue: %q", w.Detail)
	}

	// A blank epilogue is the usual trailing line break: no warning.
	if _, warnings = readTree(crlf("Content-Type: multipart/mixed; boundary=b\n\n--b\n\none\n--b--\n\n \n")); len(warnings) != 0 {
		t.Fatalf("blank epilogue: warnings %+v", warnings)
	}
}

// An unquoted parameter value runs to the next ';', so a filename with
// spaces that a sender did not quote is kept whole.
func TestReadTree_UnquotedNameWithSpaces(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, header, want string }{
		{"disposition", "Content-Type: application/pdf\nContent-Disposition: attachment; filename=My Document.pdf", "My Document.pdf"},
		{"content type", "Content-Type: application/pdf; name=My Document.pdf", "My Document.pdf"},
		{"white space before ;", "Content-Type: application/pdf\nContent-Disposition: attachment; filename=My Document.pdf  ; size=3", "My Document.pdf"},
		{"quoted value is unchanged", "Content-Type: application/pdf\nContent-Disposition: attachment; filename=\"Quoted  Name.pdf\"; x=a b", "Quoted  Name.pdf"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, warnings := readTree(crlf(tc.header + "\n\nPDF"))
			if root.Name != tc.want || string(root.Body) != "PDF" {
				t.Fatalf("name %q body %q", root.Name, root.Body)
			}
			for _, w := range warnings {
				if strings.Contains(w.Detail, "Document") || strings.Contains(w.Detail, "Name") {
					t.Fatalf("warning quotes the header: %q", w.Detail)
				}
			}
		})
	}
}
