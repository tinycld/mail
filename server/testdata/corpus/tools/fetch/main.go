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
	stalwart       = "stalwartlabs/mail-parser"
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
		note:       "Thunderbird's MIME torture test: nested multipart and message parts (MPL-2.0)."},
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
