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
