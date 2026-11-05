package mail

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/logging"
)

var parseLog = logging.ForPackage("mail")

// parseReport is what parsing had to salvage. Its caller logs it; the tree
// and lists stay so the log can name the message's structure without its
// content.
type parseReport struct {
	Warnings []parseWarning
	root     *mimePart
	lists    bodyLists
	msg      *storedMessage
}

// needsLog is true when parsing had to salvage something, or when the
// message has body text that ended up in neither body. A subject-only
// message is valid mail and logs nothing. Other text/* leaves (text/x-*) are
// attachments under RFC 8621, so they do not count.
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
		isBodyType := leaf.Type == "text/plain" || leaf.Type == "text/html"
		if isBodyType && leaf.Disposition != "attachment" && strings.TrimSpace(string(leaf.Body)) != "" {
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
	parts := cappedList{limit: maxLoggedParts}
	var walk func(p *mimePart)
	walk = func(p *mimePart) {
		parts.add(func() string {
			s := fmt.Sprintf("%d %s", p.Index, logType(p.Type))
			if cs := p.Params["charset"]; cs != "" {
				s += " charset=" + logCharset(cs)
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
			return s
		})
		for _, c := range p.Children {
			walk(c)
		}
	}
	if r.root != nil {
		walk(r.root)
	}
	return parts.String()
}

func (r parseReport) warningText() string {
	ws := cappedList{limit: maxLoggedWarnings}
	for _, w := range r.Warnings {
		ws.add(func() string { return fmt.Sprintf("%d %s: %s", w.Part, w.Kind, w.Detail) })
	}
	return ws.String()
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

// logUserID is the id to log for a session that may not have a user yet.
func logUserID(user *core.Record) string {
	if user == nil {
		return ""
	}
	return user.Id
}

const (
	maxLoggedParts    = 50
	maxLoggedWarnings = 20
)

// cappedList bounds a log field: a message can hold thousands of parts, so
// only the first limit items are formatted and the rest are only counted.
type cappedList struct {
	limit int
	items []string
	more  int
}

func (c *cappedList) add(format func() string) {
	if len(c.items) >= c.limit {
		c.more++
		return
	}
	c.items = append(c.items, format())
}

func (c *cappedList) String() string {
	s := strings.Join(c.items, "; ")
	if c.more > 0 {
		s += fmt.Sprintf("; +%d more", c.more)
	}
	return s
}

var (
	safeCharsetLabel = regexp.MustCompile(`^[a-z0-9._:-]{1,40}$`)
	safeSubtype      = regexp.MustCompile(`^[a-z0-9.+-]{1,40}$`)
	topLevelTypes    = map[string]bool{
		"text": true, "image": true, "audio": true, "video": true, "application": true,
		"multipart": true, "message": true, "font": true, "model": true,
	}
)

// logCharset is a charset label that is safe to log: the sender chooses the
// label, and it can hold a name or address. Only labels the decoder knows
// are kept; the rest is "unknown".
func logCharset(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	if !safeCharsetLabel.MatchString(label) {
		return "unknown"
	}
	if label == "utf-8" || label == "us-ascii" {
		return label
	}
	if _, err := charsetReader(label, strings.NewReader("")); err != nil {
		return "unknown"
	}
	return label
}

// logType is a media type that is safe to log: the sender chooses it, so
// only registered top-level types and plain subtypes pass.
func logType(typ string) string {
	top, sub, _ := strings.Cut(strings.ToLower(typ), "/")
	if !topLevelTypes[top] {
		return "other"
	}
	if !safeSubtype.MatchString(sub) {
		return top + "/other"
	}
	return top + "/" + sub
}
