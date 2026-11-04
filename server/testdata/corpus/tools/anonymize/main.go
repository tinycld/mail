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
	addressRe  = regexp.MustCompile(`[^\s<>,;"()]+@[^\s<>,;"()]+`)
	idRe       = regexp.MustCompile(`<[^<>]*>`)
	boundaryRe = regexp.MustCompile(`(?i)boundary\s*=\s*"?([^";\r\n]+)"?`)
	nameRe     = regexp.MustCompile(`(?i)\b((?:file)?name\*?[0-9]*\*?)\s*=\s*("[^"]*"|[^;\s]+)`)
	qpEscape   = regexp.MustCompile(`=[0-9A-Fa-f]{2}|=\r?$`)
)

// addressHeader reports whether a header holds people. Display names can be
// anywhere in the value, so the whole value is rewritten.
func addressHeader(name string) bool {
	switch name {
	case "from", "to", "cc", "bcc", "reply-to", "sender", "mail-followup-to",
		"disposition-notification-to", "return-receipt-to", "return-path":
		return true
	}
	return strings.HasPrefix(name, "resent-")
}

type anon struct {
	emails map[string]string
	files  map[string]string // original file name → fileN.ext
	ids    map[string]string // original message id → <idN@example.com>
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

func (a *anon) id(id string) string {
	if v, ok := a.ids[id]; ok {
		return v
	}
	v := fmt.Sprintf("<id%d@example.com>", len(a.ids)+1)
	a.ids[id] = v
	return v
}

func anonymize(raw []byte, keep int) []byte {
	a := &anon{emails: map[string]string{}, files: map[string]string{}, ids: map[string]string{}}
	eol := "\n"
	if bytes.Contains(raw, []byte("\r\n")) {
		eol = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	boundaries := map[string]bool{}
	var out []string
	inHeader, cte := true, ""
	placed := false // the current base64 part's placeholder is written
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if inHeader {
			// Fields are unfolded first: a name can sit on a continuation line.
			var fields []string
			for ; i < len(lines) && lines[i] != ""; i++ {
				if c := lines[i][0]; (c == ' ' || c == '\t') && len(fields) > 0 {
					fields[len(fields)-1] += lines[i]
				} else {
					fields = append(fields, lines[i])
				}
			}
			for _, f := range fields {
				name, value, _ := strings.Cut(f, ":")
				name = strings.ToLower(strings.TrimSpace(name))
				if name == "content-type" {
					for _, m := range boundaryRe.FindAllStringSubmatch(value, -1) {
						boundaries[m[1]] = true
					}
				}
				if name == "content-transfer-encoding" {
					cte = strings.ToLower(strings.TrimSpace(value))
				}
				if h, ok := a.header(f, name, value); ok {
					out = append(out, h)
				}
			}
			if i < len(lines) {
				out = append(out, "")
				inHeader = false
			}
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

// header rewrites one unfolded field. Only the headers the parser depends on
// are kept; every other header can carry private text, so it is dropped.
func (a *anon) header(field, name, value string) (string, bool) {
	switch {
	case name == "subject":
		return "Subject: Fixture message", true
	case name == "date":
		return "Date: Mon, 02 Nov 2026 12:00:00 +0000", true
	case name == "message-id" || name == "in-reply-to" || name == "references" || name == "content-id":
		return field[:len(name)] + ":" + idRe.ReplaceAllStringFunc(value, a.id), true
	case addressHeader(name):
		entries := a.addresses(value)
		return field[:len(name)] + ":" + entries, entries != ""
	case name == "mime-version" || name == "content-transfer-encoding":
		return field, true
	case name == "content-type" || name == "content-disposition":
		return a.names(emailRe.ReplaceAllStringFunc(field, a.email)), true
	}
	return "", false
}

func (a *anon) names(line string) string {
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

// addresses replaces every address in v, and so every display name, with
// "Person N <personN@example.com>". A name with no address is dropped.
func (a *anon) addresses(v string) string {
	var out []string
	for _, e := range addressRe.FindAllString(v, -1) {
		p := a.email(e)
		out = append(out, fmt.Sprintf(" Person %s <%s>", strings.TrimSuffix(strings.TrimPrefix(p, "person"), "@example.com"), p))
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
