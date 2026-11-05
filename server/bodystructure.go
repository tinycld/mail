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
// Deviations from the RFC's algorithm:
//   - The text != nil / html != nil guards in the alternative case. The RFC's
//     JavaScript would push to null (alternative > mixed[html, alternative[plain]]
//     does); such a part goes to attachments so nothing is lost.
//   - backfill: RFC 8621 defines attachments as every non-multipart part in
//     neither body list, but the algorithm can drop one.
//   - rootText: a single-part message has no delimiter to take its final line
//     break, so one break is removed (MIME framing, not content).
//
// Workarounds:
//   - relatedRootFirst: a multipart/related whose first part is not of its
//     "type" parameter is read with the part of that type first, as its root
//     (RFC 2387 §3.1): mimekit/multipart-related-mhtml.eml.

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

// structure is the RFC's parseStructure, with the deviations listed above. A nil list pointer is the RFC's
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
			children := part.Children
			if sub == "related" {
				children = relatedRootFirst(part)
			}
			structure(children, sub, inAlternative || sub == "alternative", html, text, attachments)
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

// relatedRootFirst returns a multipart/related's parts with its root first.
// RFC 2387 makes the first part the root and says the "type" parameter is the
// root's type. When the two disagree, the first part of that type is taken as
// the root: an MHTML file with its image before its HTML would otherwise show
// no body at all (mimekit/multipart-related-mhtml.eml).
func relatedRootFirst(p *mimePart) []*mimePart {
	want := strings.ToLower(strings.TrimSpace(p.Params["type"]))
	if want == "" || len(p.Children) == 0 || p.Children[0].Type == want {
		return p.Children
	}
	for i, c := range p.Children {
		if c.Type == want {
			out := append([]*mimePart{c}, p.Children[:i]...)
			return append(out, p.Children[i+1:]...)
		}
	}
	return p.Children
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

// rootText is a text part's body. The root leaf is the whole message, so no
// delimiter took its closing line break: exactly one is framing. Parts inside
// a multipart already lost theirs, so any break left is the sender's.
func rootText(p *mimePart) string {
	b := string(p.Body)
	if p.Index != 0 || p.isMultipart() {
		return b
	}
	if t := strings.TrimSuffix(b, "\n"); t != b {
		return strings.TrimSuffix(t, "\r")
	}
	return b
}

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
			texts = append(texts, rootText(p))
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
			b.WriteString(`<div style="white-space: pre-wrap">` + html.EscapeString(rootText(p)) + `</div>`)
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
