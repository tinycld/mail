package mail

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// The caps keep crafted input from costing memory far beyond its size. Each
// test passes one cap and checks that nothing is lost: what the cap stops
// reading is kept, as it is, in a part.

func countParts(p *mimePart) int {
	n := 1
	for _, c := range p.Children {
		n += countParts(c)
	}
	return n
}

func countFields(p *mimePart) int {
	n := p.Header.Len()
	for _, c := range p.Children {
		n += countFields(c)
	}
	return n
}

func warningOfKind(t *testing.T, ws []parseWarning, kind, detail string) parseWarning {
	t.Helper()
	for _, w := range ws {
		if w.Kind == kind && strings.Contains(w.Detail, detail) {
			return w
		}
	}
	t.Fatalf("no %s warning about %q in %+v", kind, detail, ws)
	return parseWarning{}
}

func TestReadTree_PartCap(t *testing.T) {
	t.Parallel()
	const head, unit = "Content-Type: multipart/mixed; boundary=b\r\n\r\n", "--b\r\n\r\nx\r\n"
	raw := []byte(head + strings.Repeat(unit, maxMIMEParts+500) + "--b--\r\n")

	root, warnings := readTree(raw)
	if got := countParts(root); got != maxMIMEParts {
		t.Fatalf("%d parts, want %d", got, maxMIMEParts)
	}
	warningOfKind(t, warnings, "bad-multipart", fmt.Sprintf("more than %d parts", maxMIMEParts))
	// The root and maxMIMEParts-2 children were read; the last child holds
	// the rest of the body, from the content of the next part on.
	rest := root.Children[len(root.Children)-1]
	off := len(head) + (maxMIMEParts-2)*len(unit) + len("--b\r\n")
	if rest.Type != "text/plain" || !bytes.Equal(rest.Body, raw[off:]) {
		t.Fatalf("rest part %s of %d bytes, want the last %d bytes", rest.Type, len(rest.Body), len(raw)-off)
	}

	_, report := parseRFC5322(raw)
	if got, want := report.structure(), fmt.Sprintf("; +%d more", maxMIMEParts-maxLoggedParts); !strings.HasSuffix(got, want) {
		t.Fatalf("structure ends %q, want %q", got[max(0, len(got)-40):], want)
	}
}

// Parts nested in parts reach the cap too, and every enclosing multipart
// keeps its own rest.
func TestReadTree_PartCapNested(t *testing.T) {
	t.Parallel()
	inner := "Content-Type: multipart/mixed; boundary=i\r\n\r\n" + strings.Repeat("--i\r\n\r\ny\r\n", maxMIMEParts) + "--i--\r\n"
	raw := []byte("Content-Type: multipart/mixed; boundary=o\r\n\r\n--o\r\n" + inner + "\r\n--o\r\n\r\nafter\r\n--o--\r\n")

	root, warnings := readTree(raw)
	if got := countParts(root); got > maxMIMEParts+1 {
		t.Fatalf("%d parts, want at most %d", got, maxMIMEParts+1)
	}
	if !hasWarning(warnings, "bad-multipart") {
		t.Fatalf("warnings %+v", warnings)
	}
	last := root.Children[len(root.Children)-1]
	if !bytes.HasSuffix(last.Body, []byte("after\r\n--o--\r\n")) {
		t.Fatalf("the outer multipart's rest was lost: %q", last.Body[max(0, len(last.Body)-40):])
	}
}

func TestParseHeaderBlock_FieldCap(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, unit, cutAt string }{
		{"fields", "X-A: b\r\n", "X-A: b\r\n"},
		// A line that is not a field before each field is skipped, so only
		// the fields count.
		{"alternating", "not a field\r\nX-A: b\r\n", "not a field\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(strings.Repeat(tc.unit, maxHeaderFields+100) + "\r\nbody\r\n")
			root, warnings := readTree(raw)
			if got := root.Header.Len(); got != maxHeaderFields {
				t.Fatalf("%d fields, want %d", got, maxHeaderFields)
			}
			warningOfKind(t, warnings, "bad-header", "kept as body")
			off := maxHeaderFields * len(tc.unit)
			if !bytes.Equal(root.Body, raw[off:]) || !bytes.HasPrefix(root.Body, []byte(tc.cutAt)) {
				t.Fatalf("body of %d bytes, want the last %d bytes", len(root.Body), len(raw)-off)
			}
		})
	}
}

func TestParseHeaderBlock_ByteCap(t *testing.T) {
	t.Parallel()
	raw := []byte("Subject: a\r\n" + strings.Repeat(" a\r\n", maxHeaderBytes/4+100) + "\r\nbody")
	h, body, problem := parseHeaderBlock(raw, maxHeaderFields)
	if problem == "" || h.Len() != 1 {
		t.Fatalf("%d fields, problem %q", h.Len(), problem)
	}
	if read := len(raw) - len(body); read > maxHeaderBytes || !bytes.HasSuffix(raw, body) || !bytes.HasPrefix(body, []byte(" a\r\n")) {
		t.Fatalf("read %d bytes of header, kept %d as body", read, len(body))
	}
	// One line longer than the cap is body too.
	raw = []byte("Subject: " + strings.Repeat("a", maxHeaderBytes) + "\r\n\r\nbody")
	if h, body, problem = parseHeaderBlock(raw, maxHeaderFields); h.Len() != 0 || !bytes.Equal(body, raw) || problem == "" {
		t.Fatalf("long line: %d fields, %d bytes of body, problem %q", h.Len(), len(body), problem)
	}
}

// The fields of all parts together are capped too: many parts each under
// the cap of one header block cost as much as one huge block.
func TestReadTree_MessageFieldCap(t *testing.T) {
	t.Parallel()
	const perPart = 1000
	n := maxMessageFields/perPart + 5
	part := "--b\r\n" + strings.Repeat("X-A: b\r\n", perPart) + "\r\nx\r\n"
	raw := []byte("Content-Type: multipart/mixed; boundary=b\r\n\r\n" + strings.Repeat(part, n) + "--b--\r\n")

	root, warnings := readTree(raw)
	if got := countFields(root); got != maxMessageFields {
		t.Fatalf("%d fields in the message, want %d", got, maxMessageFields)
	}
	warningOfKind(t, warnings, "bad-header", "kept as body")
	if len(root.Children) != n {
		t.Fatalf("%d parts, want %d", len(root.Children), n)
	}
	// The root's one field took one of the budget, so the part that ran out
	// keeps its last field in its body, and the parts after it keep all of
	// theirs there.
	content := strings.TrimSuffix(strings.TrimPrefix(part, "--b\r\n"), "\r\n")
	ranOut := root.Children[maxMessageFields/perPart-1]
	if ranOut.Header.Len() != perPart-1 || string(ranOut.Body) != "X-A: b\r\n\r\nx" {
		t.Fatalf("the part that ran out has %d fields and a body of %d bytes", ranOut.Header.Len(), len(ranOut.Body))
	}
	for _, p := range root.Children[maxMessageFields/perPart:] {
		if p.Header.Len() != 0 || string(p.Body) != content {
			t.Fatalf("part %d has %d fields and a body of %d bytes", p.Index, p.Header.Len(), len(p.Body))
		}
	}
}

// Delimiters are found with bytes.Index, so a deep nest of multiparts over a
// long run of short lines is read in time linear in its size; reading each
// line once per level made this 15 times slower.
func TestReadTree_DeepNestOverManyLines(t *testing.T) {
	t.Parallel()
	levels := maxMIMEDepth - 1
	var b strings.Builder
	for i := 0; i < levels; i++ {
		fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=b%d\r\n\r\n--b%d\r\n", i, i)
	}
	b.WriteString("Content-Type: text/plain\r\n\r\n")
	lines := 1_000_000
	b.WriteString(strings.Repeat("x\r\n", lines))
	b.WriteString("end")
	for i := levels - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "\r\n--b%d--", i)
	}
	b.WriteString("\r\n")

	root, warnings := readTree([]byte(b.String()))
	if len(warnings) != 0 {
		t.Fatalf("warnings %+v", warnings)
	}
	ls := leaves(root)
	if len(ls) != 1 || countParts(root) != levels+1 {
		t.Fatalf("%d leaves, %d parts", len(ls), countParts(root))
	}
	if want := strings.Repeat("x\r\n", lines) + "end"; string(ls[0].Body) != want {
		t.Fatalf("leaf of %d bytes, want %d", len(ls[0].Body), len(want))
	}
}

// splitByLines is the splitter as it was before delimiters were searched
// for: it reads every line. The search must find exactly what it found.
func splitByLines(body []byte, boundary string) (preamble []byte, parts [][]byte, closed, found bool) {
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

func TestSplitMultipart_MatchesLineScan(t *testing.T) {
	t.Parallel()
	tokens := []string{"--b", "--b--", "--bx", "x--b", "\r\n", "\n", "\r", " ", "\t", "x", "-", "--"}
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 20_000; i++ {
		var b strings.Builder
		for n := rng.IntN(16); n > 0; n-- {
			b.WriteString(tokens[rng.IntN(len(tokens))])
		}
		body := []byte(b.String())
		pre, parts, closed, found := splitMultipart(body, "b")
		wantPre, wantParts, wantClosed, wantFound := splitByLines(body, "b")
		if !bytes.Equal(pre, wantPre) || !slices.EqualFunc(parts, wantParts, bytes.Equal) || closed != wantClosed || found != wantFound {
			t.Fatalf("%q: got %q %q %v %v, want %q %q %v %v", body, pre, parts, closed, found, wantPre, wantParts, wantClosed, wantFound)
		}
	}
}
