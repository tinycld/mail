package mail

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var collapseWhitespace = regexp.MustCompile(`\s+`)

// stripHTMLToText removes all HTML tags and collapses whitespace,
// returning plain text suitable for FTS indexing.
func stripHTMLToText(html string) string {
	text := bluemonday.StrictPolicy().Sanitize(html)
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&#160;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&#34;", `"`)
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = collapseWhitespace.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func sanitizeEmailHTML(raw string) string {
	p := bluemonday.UGCPolicy()
	p.AllowElements("table", "thead", "tbody", "tfoot", "tr", "td", "th",
		"caption", "col", "colgroup")
	p.AllowAttrs("style").OnElements("div", "span", "p", "td", "th", "table",
		"tr", "h1", "h2", "h3", "h4", "h5", "h6", "img", "a")
	p.AllowAttrs("width", "height", "cellpadding", "cellspacing", "border",
		"align", "valign", "bgcolor", "colspan", "rowspan").OnElements(
		"table", "td", "th", "tr", "img")
	p.AllowAttrs("src", "alt").OnElements("img")
	p.AllowURLSchemeWithCustomPolicy("cid", func(_ *url.URL) bool { return true })
	return sanitizeInlineStyles(p.Sanitize(raw))
}

// styleAttr matches a style="..." or style='...' attribute in the sanitized
// HTML so its CSS can be post-processed. bluemonday allows the style attribute
// but does not sanitize CSS, so this pass neutralizes the two vectors it lets
// through: url() references and overlay positioning.
var styleAttr = regexp.MustCompile(`(?i)\bstyle\s*=\s*("([^"]*)"|'([^']*)')`)

// cssURL matches a CSS url(...) reference in any casing/quoting. It is the
// tracking-pixel vector: bluemonday only allows <img src> through the
// client-side image proxy, but background:url('http://tracker/pixel') in an
// inline style is fetched directly by the rendering iframe/WebView, leaking the
// recipient's IP and confirming the message was opened (read receipt).
var cssURL = regexp.MustCompile(`(?i)url\s*\(\s*(?:"[^"]*"|'[^']*'|[^)]*)\)`)

// cssDangerousPosition matches position declarations that let an email break
// out of its container and overlay the surrounding UI (phishing). Only static
// and relative positioning is safe inside the sandboxed body.
var cssDangerousPosition = regexp.MustCompile(`(?i)position\s*:\s*(fixed|absolute|sticky)`)

// sanitizeInlineStyles strips url() references and dangerous position values
// from every inline style attribute while preserving benign declarations
// (colors, fonts, spacing, widths). It runs after bluemonday because
// bluemonday does not sanitize CSS at all.
func sanitizeInlineStyles(html string) string {
	return styleAttr.ReplaceAllStringFunc(html, func(match string) string {
		m := styleAttr.FindStringSubmatch(match)
		// m[1] is the full quoted value; m[2]/m[3] the double/single-quoted
		// inner CSS. Preserve whichever quote style the original used.
		quote := `"`
		css := m[2]
		if strings.HasPrefix(m[1], "'") {
			quote = "'"
			css = m[3]
		}
		return "style=" + quote + sanitizeCSS(css) + quote
	})
}

// sanitizeCSS neutralizes the unsafe parts of a single style attribute value.
func sanitizeCSS(css string) string {
	css = cssURL.ReplaceAllString(css, "")
	css = cssDangerousPosition.ReplaceAllString(css, "")
	return css
}
