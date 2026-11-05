package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/textproto"
)

// readTree salvages a message's whole MIME tree from any input (design,
// "Step 1"). It never fails and never drops bytes it could keep: what it
// cannot decode it keeps undecoded, and every problem is a warning.
//
// It parses headers, boundaries and encodings itself instead of using
// go-message's entity reader, which refuses a malformed header line and
// decodes transfer encodings and charsets with no way to keep a partial
// result. It still fills a go-message textproto.Header, so go-message's
// mail.Header helpers read the fields.

// maxMIMEDepth bounds how deep readTree follows nested multiparts, so a
// crafted message cannot exhaust the stack.
const maxMIMEDepth = 50

type mimePart struct {
	Index       int
	Header      textproto.Header
	Type        string
	Params      map[string]string
	Disposition string
	Name        string
	ContentID   string
	Body        []byte
	RawSize     int
	Children    []*mimePart
}

func (p *mimePart) isMultipart() bool { return strings.HasPrefix(p.Type, "multipart/") }

type parseWarning struct {
	Part   int
	Kind   string
	Detail string
}

func readTree(raw []byte) (*mimePart, []parseWarning) {
	// A final CR with no LF after it is the first half of a line break that
	// the message was cut off in, not content (stalwart/legacy/049.eml).
	raw = bytes.TrimSuffix(raw, []byte("\r"))
	r := &treeReader{}
	return r.read(raw, 0, "text/plain"), r.warnings
}

// leaves returns the tree's non-multipart parts, depth-first.
func leaves(root *mimePart) []*mimePart {
	if !root.isMultipart() {
		return []*mimePart{root}
	}
	var out []*mimePart
	for _, c := range root.Children {
		out = append(out, leaves(c)...)
	}
	return out
}

type treeReader struct {
	next     int
	warnings []parseWarning
}

func (r *treeReader) warn(part int, kind, format string, args ...any) {
	r.warnings = append(r.warnings, parseWarning{Part: part, Kind: kind, Detail: fmt.Sprintf(format, args...)})
}

// read builds the part whose header and body are b. defaultType is the type
// of a part with no Content-Type: text/plain, or message/rfc822 inside a
// multipart/digest (RFC 2046 §5.1.5).
func (r *treeReader) read(b []byte, depth int, defaultType string) *mimePart {
	p := &mimePart{Index: r.next}
	r.next++

	h, body, problem := parseHeaderBlock(b)
	if problem != "" {
		r.warn(p.Index, "bad-header", "%s", problem)
	}
	p.Header = h
	p.RawSize = len(bytes.TrimSpace(body))

	ct := p.Header.Get("Content-Type")
	typ, params, ok := parseContentType(ct, defaultType)
	if !ok {
		r.warn(p.Index, "bad-content-type", "Content-Type of %d bytes read as %s", len(ct), logType(typ))
	}
	p.Type, p.Params = typ, params
	p.Disposition, p.Name = r.disposition(p)
	p.ContentID = strings.TrimSpace(p.Header.Get("Content-Id"))

	if p.isMultipart() {
		r.readChildren(p, body, depth)
	} else {
		r.decodeLeaf(p, p.Header.Get("Content-Transfer-Encoding"), body)
	}
	return p
}

func (r *treeReader) readChildren(p *mimePart, body []byte, depth int) {
	if depth >= maxMIMEDepth {
		r.warn(p.Index, "too-deep", "multipart nested deeper than %d levels; kept as one file", maxMIMEDepth)
		r.keepAsLeaf(p, "application/octet-stream", body)
		return
	}
	preamble, parts, closed, found := splitMultipart(body, p.Params["boundary"])
	if !found {
		r.warn(p.Index, "bad-multipart", "no delimiter for the boundary of %d bytes; kept as one part", len(p.Params["boundary"]))
		typ := "application/octet-stream"
		if looksLikeText(body) {
			typ = "text/plain"
		}
		r.keepAsLeaf(p, typ, body)
		return
	}
	if !closed {
		r.warn(p.Index, "truncated", "no closing delimiter; the last part runs to the end of the message")
	}
	if len(parts) == 0 && len(bytes.TrimSpace(preamble)) > 0 {
		// RFC 2046 drops the preamble, but a multipart with no parts has
		// nothing else to show.
		child := &mimePart{Index: r.next, Type: "text/plain", Params: map[string]string{}, RawSize: len(bytes.TrimSpace(preamble))}
		r.next++
		r.decodeLeaf(child, "", preamble)
		p.Children = []*mimePart{child}
		return
	}
	childType := "text/plain"
	if p.Type == "multipart/digest" {
		childType = "message/rfc822"
	}
	for _, raw := range parts {
		p.Children = append(p.Children, r.read(raw, depth+1, childType))
	}
}

// keepAsLeaf turns a multipart that cannot be split into one leaf holding
// its raw bytes.
func (r *treeReader) keepAsLeaf(p *mimePart, typ string, body []byte) {
	p.Type = typ
	p.Params = map[string]string{}
	p.Children = nil
	r.decodeLeaf(p, "", body)
}

func (r *treeReader) decodeLeaf(p *mimePart, cte string, body []byte) {
	decoded, problem := decodeTransfer(cte, body)
	if problem != "" {
		r.warn(p.Index, "bad-encoding", "%s", problem)
	}
	if strings.HasPrefix(p.Type, "text/") {
		text, kind, problem := decodeText(p.Type, p.Params["charset"], decoded)
		if problem != "" {
			r.warn(p.Index, kind, "%s", problem)
		}
		decoded = text
	}
	p.Body = decoded
}

func (r *treeReader) disposition(p *mimePart) (string, string) {
	raw := p.Header.Get("Content-Disposition")
	disp, params := "", map[string]string{}
	if strings.TrimSpace(raw) != "" {
		d, ps, err := mime.ParseMediaType(raw)
		if err != nil {
			d, ps = lenientMediaType(raw, dispositionToken)
			r.warn(p.Index, "bad-header-param", "Content-Disposition of %d bytes read leniently", len(raw))
		}
		disp, params = strings.ToLower(d), ps
		// RFC 2183: an unknown disposition is treated as attachment. A value
		// with no type at all (";" then a filename) names no disposition, so
		// the part keeps the one its place gives it (stalwart/malformed/017.eml).
		if disp != "" && disp != "inline" && disp != "attachment" {
			disp = "attachment"
		}
	}
	name := params["filename"]
	if name == "" {
		name = p.Params["name"]
	}
	return disp, r.decodeWords(p.Index, name)
}

var wordDecoder = mime.WordDecoder{CharsetReader: charsetReader}

// charsetReader is go-message's charset.Reader plus UTF-7 (RFC 2152), which
// golang.org/x/text does not have and old Outlook versions send.
// A header word or parameter has no warning path through this reader, so
// ill-formed UTF-7 there is decoded as decodeUTF7 keeps it, without a warning;
// decodeText reports it for bodies.
func charsetReader(label string, r io.Reader) (io.Reader, error) {
	if isUTF7(label) {
		b, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		out, _ := decodeUTF7(b)
		return bytes.NewReader(out), nil
	}
	return charset.Reader(label, r)
}

func isUTF7(label string) bool {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "utf-7", "utf7", "unicode-1-1-utf-7", "csunicode11utf7":
		return true
	}
	return false
}

// decodeUTF7 decodes RFC 2152 UTF-7. "+" opens a run of modified base64
// that holds UTF-16 code units; the first character outside the base64
// alphabet ends it, and is kept unless it is "-". "+-" is a literal "+".
// Fewer than 6 bits left at the end of a run are padding; a lone surrogate
// becomes U+FFFD.
//
// Ill-formed input is kept, and wellFormed is false: a "+" whose run holds
// no whole code unit ("1 + 1", "C++", "x+y") is written out as it is, since
// it is almost always a literal plus a sender did not escape, and 6 or more
// bits left over become U+FFFD.
func decodeUTF7(b []byte) (out []byte, wellFormed bool) {
	out = make([]byte, 0, len(b))
	wellFormed = true
	for i := 0; i < len(b); {
		if b[i] != '+' {
			out = append(out, b[i])
			i++
			continue
		}
		start := i
		i++
		if i < len(b) && b[i] == '-' {
			out = append(out, '+')
			i++
			continue
		}
		var units []uint16
		var bits uint32
		n := 0
		for ; i < len(b); i++ {
			v := strings.IndexByte(base64Alphabet, b[i])
			if v < 0 {
				break
			}
			bits = bits<<6 | uint32(v)
			n += 6
			if n >= 16 {
				n -= 16
				units = append(units, uint16(bits>>n))
				bits &= 1<<n - 1
			}
		}
		if len(units) == 0 {
			wellFormed = false
			out = append(out, b[start:i]...)
			continue
		}
		out = append(out, string(utf16.Decode(units))...)
		if n >= 6 {
			wellFormed = false
			out = append(out, "\uFFFD"...)
		}
		if i < len(b) && b[i] == '-' {
			i++
		}
	}
	return out, wellFormed
}

const base64Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

// decodeWords decodes RFC 2047 encoded words, keeping the value as written
// when it cannot.
func (r *treeReader) decodeWords(part int, v string) string {
	if !strings.Contains(v, "=?") {
		return v
	}
	out, err := wordDecoder.DecodeHeader(v)
	if err != nil {
		r.warn(part, "bad-header-param", "could not decode a parameter of %d bytes", len(v))
		return v
	}
	return out
}

var headerFieldLine = regexp.MustCompile(`^([!-9;-~]+)[ \t]*:(.*)$`)

// parseHeaderBlock reads the header fields at the start of b and returns
// them with the body after the blank line that ends them. It never fails.
//
// A line that is neither a field nor a continuation is skipped, with its
// continuations, only when a field comes right after it: the header goes on
// past it. Any other such line ends the header there, and the body starts at
// that line, so no byte is dropped: a body line right after the fields
// ("Hi Bob: lunch?", or "Dear Bob," over an indented line) is body, even
// though stalwart skips such a line (known_deviations.txt). Input with no
// readable field before such a line has no header and is all body (design,
// "Header block cannot be read").
func parseHeaderBlock(b []byte) (textproto.Header, []byte, string) {
	var fields []headerLine
	var problem string
	for pos := 0; pos < len(b); {
		line, next := lineAt(b, pos)
		switch {
		case len(line) == 0:
			return headerOf(fields), b[next:], problem
		case isContinuation(line):
			// A skipped line always has a field right after it, so a
			// continuation here belongs to the last field.
			if len(fields) == 0 {
				return textproto.Header{}, b, ""
			}
			// RFC 5322 unfolding removes the line break and keeps the white
			// space. Appending to a byte slice keeps a message of many
			// continuation lines linear; string concatenation copied the whole
			// value per line.
			fields[len(fields)-1].value = append(fields[len(fields)-1].value, line...)
		default:
			if m := headerFieldLine.FindSubmatch(line); m != nil {
				fields = append(fields, headerLine{string(m[1]), append([]byte(nil), m[2]...)})
				break
			}
			// Problems name a line's length, never its text: a header line
			// can hold a name or an address, and warnings reach the log.
			if following, _ := lineAt(b, next); next < len(b) && headerFieldLine.Match(following) {
				// Also the first line: an mbox "From " line comes before the fields.
				problem = fmt.Sprintf("skipped a header line of %d bytes that is not a field", len(line))
				break
			}
			if len(fields) == 0 {
				return textproto.Header{}, b, ""
			}
			return headerOf(fields), b[pos:], fmt.Sprintf("header ended early at a line of %d bytes that is not a field", len(line))
		}
		pos = next
	}
	return headerOf(fields), nil, problem
}

func isContinuation(line []byte) bool {
	return len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
}

type headerLine struct {
	name  string
	value []byte
}

// headerOf builds a go-message header from fields in message order. The
// header stores fields last-first, so they are added in reverse: Get then
// returns the first field of a name, and Fields iterates in message order,
// as go-message's own reader leaves them.
func headerOf(fields []headerLine) textproto.Header {
	var h textproto.Header
	for i := len(fields) - 1; i >= 0; i-- {
		h.Add(strings.TrimSpace(fields[i].name), strings.TrimSpace(string(fields[i].value)))
	}
	return h
}

// lineAt returns the line at pos without its line break, and where the next
// line starts.
func lineAt(b []byte, pos int) ([]byte, int) {
	end := bytes.IndexByte(b[pos:], '\n')
	if end < 0 {
		return bytes.TrimSuffix(b[pos:], []byte("\r")), len(b)
	}
	return bytes.TrimSuffix(b[pos:pos+end], []byte("\r")), pos + end + 1
}

// parseContentType reads a Content-Type value. ok is false when it had to
// be read leniently; a value with no usable type falls back to defaultType,
// keeping whatever parameters (a boundary) could be read.
func parseContentType(v, defaultType string) (string, map[string]string, bool) {
	if strings.TrimSpace(v) == "" {
		return defaultType, map[string]string{}, true
	}
	// mime.ParseMediaType accepts a type with no subtype ("multipart"),
	// which RFC 2045 does not: that value is read leniently, so the part gets
	// the default type (stalwart/malformed/023.eml).
	if mt, params, err := mime.ParseMediaType(v); err == nil && strings.Contains(mt, "/") {
		return mt, params, true
	}
	mt, params := lenientMediaType(v, mediaTypeToken)
	if mt == "" {
		return defaultType, params, false
	}
	return mt, params, false
}

var lenientParam = regexp.MustCompile(`(?i)([a-z0-9!#$&^_.+-]+)(\*[0-9]*\*?)?\s*=\s*("(?:[^"\\]|\\.)*"?|[^;\s]*)`)

var (
	mediaTypeToken   = regexp.MustCompile(`(?i)^\s*([a-z0-9!#$&^_.+-]+/[a-z0-9!#$&^_.+-]+)`)
	dispositionToken = regexp.MustCompile(`(?i)^\s*([a-z0-9!#$&^_.+-]+)`)
)

// lenientMediaType reads a Content-Type or Content-Disposition value that
// mime.ParseMediaType refuses: the type is the leading token (so a missing
// ';' before the parameters still gives the right type), and each name=value
// pair in the rest of the value is a parameter. A value with no leading token
// gives an empty type but keeps its parameters. RFC 2231 continuations
// (name*0, name*1) are joined and extended values (name*=charset'lang'text)
// are percent-decoded; a value that cannot be decoded stays as written.
func lenientMediaType(v string, token *regexp.Regexp) (string, map[string]string) {
	typ, rest := "", v
	if loc := token.FindStringSubmatchIndex(v); loc != nil {
		typ, rest = v[loc[2]:loc[3]], v[loc[1]:]
	}
	params := map[string]string{}
	pieces := map[string][]string{}
	for _, m := range lenientParam.FindAllStringSubmatch(rest, -1) {
		name, ext, value := strings.ToLower(m[1]), m[2], unquoteParam(m[3])
		if strings.HasSuffix(ext, "*") {
			value = decodeExtValue(value)
		}
		if ext != "" && ext != "*" {
			pieces[name] = append(pieces[name], value)
			continue
		}
		params[name] = value
	}
	for name, ps := range pieces {
		if _, ok := params[name]; !ok {
			params[name] = strings.Join(ps, "")
		}
	}
	return strings.ToLower(strings.TrimSpace(typ)), params
}

func unquoteParam(v string) string {
	if strings.HasPrefix(v, `"`) {
		v = strings.TrimSuffix(strings.TrimPrefix(v, `"`), `"`)
		v = strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(v)
	}
	return strings.TrimSpace(v)
}

// decodeExtValue decodes an RFC 2231 extended value, charset'lang'%XX…, or
// a continuation piece that is only %XX…, keeping it as written on failure.
func decodeExtValue(v string) string {
	cs, encoded := "", v
	if parts := strings.SplitN(v, "'", 3); len(parts) == 3 {
		cs, encoded = parts[0], parts[2]
	}
	b, err := url.PathUnescape(encoded)
	if err != nil {
		return v
	}
	if cs == "" || strings.EqualFold(cs, "utf-8") || strings.EqualFold(cs, "us-ascii") {
		return b
	}
	rd, err := charsetReader(cs, strings.NewReader(b))
	if err != nil {
		return b
	}
	out, err := io.ReadAll(rd)
	if err != nil {
		return b
	}
	return string(out)
}

// splitMultipart cuts a multipart body at the delimiter lines of boundary. A
// delimiter line is "--boundary", or "--boundary--" for the close, at the
// start of a line, with only white space after it; the line break before it
// belongs to the delimiter (RFC 2046 §5.1.1). found is false when no
// delimiter line exists. closed is false when the close delimiter is missing;
// the last part then runs to the end of body.
func splitMultipart(body []byte, boundary string) (preamble []byte, parts [][]byte, closed, found bool) {
	if boundary == "" {
		return nil, nil, false, false
	}
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

// delimiterKind is 1 for an open delimiter line, 2 for the close, 0 otherwise.
func delimiterKind(line, delim []byte) int {
	line = bytes.TrimRight(line, " \t\r")
	if !bytes.HasPrefix(line, delim) {
		return 0
	}
	switch rest := line[len(delim):]; {
	case len(rest) == 0:
		return 1
	case bytes.Equal(rest, []byte("--")):
		return 2
	}
	return 0
}

// lineBreakStart is where the line break before the line at pos begins.
func lineBreakStart(b []byte, pos int) int {
	switch {
	case pos >= 2 && b[pos-2] == '\r' && b[pos-1] == '\n':
		return pos - 2
	case pos >= 1 && b[pos-1] == '\n':
		return pos - 1
	}
	return pos
}

// looksLikeText reports whether b reads as text: at most 1% control bytes in
// its first 4 KB.
func looksLikeText(b []byte) bool {
	sample := b[:min(len(b), 4096)]
	control := 0
	for _, c := range sample {
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' && c != '\f' {
			control++
		}
	}
	return control*100 <= len(sample)
}

func decodeTransfer(cte string, body []byte) ([]byte, string) {
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "", "7bit", "8bit", "binary":
		return body, ""
	case "base64":
		return decodeBase64Lenient(body)
	case "quoted-printable":
		out := decodeQuotedPrintable(body)
		if len(out) == 0 && len(bytes.TrimSpace(body)) > 0 {
			return body, "quoted-printable body held nothing decodable; kept as is"
		}
		return out, ""
	default:
		return body, fmt.Sprintf("unknown Content-Transfer-Encoding of %d bytes; kept as is", len(cte))
	}
}

// decodeBase64Lenient decodes base64 that may carry line breaks, stray
// characters or broken padding, as lenient mail readers do: it keeps only
// the base64 alphabet, decodes each run between padding, and drops a
// trailing character too short to hold a byte. With nothing decodable the
// raw body is kept.
func decodeBase64Lenient(body []byte) ([]byte, string) {
	var out, run []byte
	junk := 0
	flush := func() {
		if len(run)%4 == 1 {
			run = run[:len(run)-1]
		}
		if len(run) > 0 {
			dst := make([]byte, base64.RawStdEncoding.DecodedLen(len(run)))
			n, _ := base64.RawStdEncoding.Decode(dst, run)
			out = append(out, dst[:n]...)
		}
		run = run[:0]
	}
	for _, c := range body {
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '+', c == '/':
			run = append(run, c)
		case c == '=':
			flush()
		case c == ' ', c == '\t', c == '\r', c == '\n':
		default:
			junk++
		}
	}
	flush()
	if len(out) == 0 && len(bytes.TrimSpace(body)) > 0 {
		return body, "base64 body held nothing decodable; kept as is"
	}
	if junk > 0 {
		return out, fmt.Sprintf("dropped %d bytes outside the base64 alphabet", junk)
	}
	return out, ""
}

// decodeQuotedPrintable decodes quoted-printable leniently: "=XX" with two
// hex digits is a byte, "=" before a line break is a soft break, and any
// other "=" stays as written, so broken encoding never loses text.
func decodeQuotedPrintable(body []byte) []byte {
	out := make([]byte, 0, len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '=' {
			out = append(out, c)
			continue
		}
		if i+2 < len(body) && isHex(body[i+1]) && isHex(body[i+2]) {
			out = append(out, unhex(body[i+1])<<4|unhex(body[i+2]))
			i += 2
			continue
		}
		j := i + 1
		for j < len(body) && (body[j] == ' ' || body[j] == '\t') {
			j++
		}
		switch {
		case j == len(body):
			i = j - 1
		case body[j] == '\n':
			i = j
		case body[j] == '\r' && j+1 < len(body) && body[j+1] == '\n':
			i = j + 1
		default:
			out = append(out, c)
		}
	}
	return out
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f'
}

func unhex(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}

var metaCharset = regexp.MustCompile(`(?i)<meta[^>]+charset\s*=\s*["']?\s*([a-z0-9_.:-]+)`)

// decodeMeta reads HTML bytes with the charset of their <meta> tag. It is a
// last resort for bytes that are not UTF-8, so a stale tag on UTF-8 text
// never overrides the bytes.
func decodeMeta(typ string, b []byte) ([]byte, bool) {
	if typ != "text/html" {
		return nil, false
	}
	m := metaCharset.FindSubmatch(b[:min(len(b), 2048)])
	if m == nil {
		return nil, false
	}
	rd, err := charsetReader(strings.ToLower(string(m[1])), bytes.NewReader(b))
	if err != nil {
		return nil, false
	}
	out, err := io.ReadAll(rd)
	if err != nil {
		return nil, false
	}
	return toValidUTF8(out), true
}

// decodeText turns a text part's bytes into UTF-8. A declared charset that
// go-message's charset package knows is used. With no charset, or one nobody
// knows, bytes that are valid UTF-8 are used as they are; otherwise HTML is
// read with its <meta charset> and anything else as Windows-1252, which maps
// every byte to a character. kind and problem say what had to be guessed.
func decodeText(typ, label string, b []byte) ([]byte, string, string) {
	label = strings.ToLower(strings.TrimSpace(label))
	switch label {
	case "":
		if utf8.Valid(b) {
			return b, "", ""
		}
		if out, ok := decodeMeta(typ, b); ok {
			return out, "", ""
		}
		return windows1252(b), "unknown-charset", "no charset and not UTF-8; read as Windows-1252"
	case "us-ascii", "ascii":
		if utf8.Valid(b) {
			return b, "", ""
		}
		return windows1252(b), "unknown-charset", fmt.Sprintf("charset %q but not UTF-8; read as Windows-1252", logCharset(label))
	case "utf-8", "utf8":
		if utf8.Valid(b) {
			return b, "", ""
		}
		return toValidUTF8(b), "invalid-utf8", "declared UTF-8 but holds invalid sequences; replaced them with U+FFFD"
	}
	if isUTF7(label) {
		out, wellFormed := decodeUTF7(b)
		switch {
		case !wellFormed:
			return toValidUTF8(out), "bad-encoding", "ill-formed UTF-7; kept as written"
		case !utf8.Valid(out):
			return toValidUTF8(out), "invalid-utf8", "UTF-7 text holds bytes outside it; replaced them with U+FFFD"
		}
		return out, "", ""
	}
	if rd, err := charsetReader(label, bytes.NewReader(b)); err == nil {
		if decoded, err := io.ReadAll(rd); err == nil {
			if utf8.Valid(decoded) {
				return decoded, "", ""
			}
			return toValidUTF8(decoded), "invalid-utf8", fmt.Sprintf("%s text decoded to invalid UTF-8; replaced the bad sequences", logCharset(label))
		}
	}
	if utf8.Valid(b) {
		return b, "unknown-charset", fmt.Sprintf("unknown charset %q; the bytes are UTF-8 and were used as such", logCharset(label))
	}
	if out, ok := decodeMeta(typ, b); ok {
		return out, "unknown-charset", fmt.Sprintf("unknown charset %q; read with the charset of the HTML meta tag", logCharset(label))
	}
	return windows1252(b), "unknown-charset", fmt.Sprintf("unknown charset %q; read as Windows-1252", logCharset(label))
}

func windows1252(b []byte) []byte {
	if rd, err := charset.Reader("windows-1252", bytes.NewReader(b)); err == nil {
		if out, err := io.ReadAll(rd); err == nil {
			return toValidUTF8(out)
		}
	}
	return toValidUTF8(b)
}

// toValidUTF8 replaces each maximal subpart of an invalid sequence with one
// U+FFFD, as Unicode (§3.9, "U+FFFD Substitution of Maximal Subparts") and
// WHATWG recommend. bytes.ToValidUTF8 replaces a whole run of bad bytes with
// one, so three stray bytes would read as one (stalwart/thirdparty/002.eml).
func toValidUTF8(b []byte) []byte {
	if utf8.Valid(b) {
		return b
	}
	out := make([]byte, 0, len(b)+8)
	for i := 0; i < len(b); {
		if r, size := utf8.DecodeRune(b[i:]); r != utf8.RuneError || size > 1 {
			out = append(out, b[i:i+size]...)
			i += size
			continue
		}
		out = append(out, "\uFFFD"...)
		i += maximalSubpart(b[i:])
	}
	return out
}

// maximalSubpart is the length of the longest start of b that could begin a
// well-formed UTF-8 sequence, at least 1. b does not start with one.
func maximalSubpart(b []byte) int {
	lo, hi, need := byte(0x80), byte(0xBF), 0
	switch c := b[0]; {
	case c >= 0xC2 && c <= 0xDF:
		need = 1
	case c == 0xE0:
		lo, need = 0xA0, 2
	case c == 0xED:
		hi, need = 0x9F, 2
	case c >= 0xE1 && c <= 0xEF:
		need = 2
	case c == 0xF0:
		lo, need = 0x90, 3
	case c == 0xF4:
		hi, need = 0x8F, 3
	case c >= 0xF1 && c <= 0xF3:
		need = 3
	}
	n := 1
	for ; n <= need && n < len(b); n++ {
		if b[n] < lo || b[n] > hi {
			break
		}
		lo, hi = 0x80, 0xBF
	}
	return n
}
