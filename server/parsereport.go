package mail

import (
	"fmt"
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

// logUserID is the id to log for a session that may not have a user yet.
func logUserID(user *core.Record) string {
	if user == nil {
		return ""
	}
	return user.Id
}
