package mail

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	for _, problem := range losesNothingProblems(raw) {
		t.Error(problem)
	}
}

// losesNothingProblems returns the salvage guarantees one message breaks. It
// takes no *testing.T so the fuzz test can run it in a goroutine it abandons
// on timeout. Problems name part numbers, types and sizes, never content.
func losesNothingProblems(raw []byte) []string {
	var problems []string
	root, _ := readTree(raw)
	l := parseStructure(root)
	listed := map[*mimePart]bool{}
	for _, list := range [][]*mimePart{l.text, l.html, l.attachments} {
		for _, p := range list {
			listed[p] = true
		}
	}
	isBody := map[*mimePart]bool{}
	for _, list := range [][]*mimePart{l.text, l.html} {
		for _, p := range list {
			isBody[p] = true
		}
	}
	textLeaf := false
	for _, leaf := range leaves(root) {
		if len(leaf.Body) > 0 && !listed[leaf] {
			problems = append(problems, fmt.Sprintf("part %d (%s, %d bytes) is in no list", leaf.Index, leaf.Type, len(leaf.Body)))
		}
		if leaf.RawSize > 0 && len(leaf.Body) == 0 {
			problems = append(problems, fmt.Sprintf("part %d (%s) had %d bytes and kept none", leaf.Index, leaf.Type, leaf.RawSize))
		}
		// Only text/plain and text/html can be bodies in RFC 8621; text/x-* is a file.
		// Only parts parseStructure put in a body list count: a text part RFC 8621 sends to the files
		// (a second part of a multipart/related, a named part after the first) is no body. That
		// classification is gated by TestCorpus_MatchesExpected and the RFC unit tests; this
		// property gates flatten.
		if (leaf.Type == "text/plain" || leaf.Type == "text/html") && leaf.Disposition != "attachment" && isBody[leaf] && len(bytes.TrimSpace(leaf.Body)) > 0 {
			textLeaf = true
		}
	}
	msg, _ := parseRFC5322(raw)
	if textLeaf && strings.TrimSpace(msg.TextBody) == "" && strings.TrimSpace(msg.HTMLBody) == "" {
		problems = append(problems, "the message has text, but both bodies are empty")
	}
	again, _ := parseRFC5322(raw)
	if !reflect.DeepEqual(msg, again) {
		problems = append(problems, "parsing twice gave different results")
	}
	return problems
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
		Size *int64 `json:"size"` // decoded bytes; checked when present
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
				if got := msg.Attachments[i].Size; a.Size != nil && got != *a.Size {
					t.Errorf("attachment %d size = %d, want %d", i, got, *a.Size)
				}
			}
		})
	}
}
