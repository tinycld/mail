package mail

import (
	"net/url"

	"github.com/microcosm-cc/bluemonday"
)

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
	return p.Sanitize(raw)
}
