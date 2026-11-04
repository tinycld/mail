# MIME parsing by RFC 8621, tested against a corpus — design

Date: 2026-11-04
Status: draft, for review

## Why

`parseRFC5322` (`server/imap_rfc5322.go`) turns a raw message into the
`storedMessage` that IMAP APPEND, SMTP submission, the port-25 listener and
the SMTP provider path store or send. It has failed three times on ordinary
mail from a widely used client, each time with a different fault:

1. A body split around a file kept only its last, empty part, and the file
   marked `inline` was dropped (mail v0.10.1).
2. HTML that sanitizes to nothing made the store fail (mail v0.10.1).
3. A message with a subject and two files and no text part (Apple Mail:
   `multipart/mixed` of an `application/octet-stream` attachment and an
   inline PDF) parses correctly, but SMTP submission sends Postmark an empty
   `TextBody` and no `HtmlBody`, which Postmark refuses (ErrorCode 300), so
   the client gets a 451 and retries for ever. This one is still live.

A fourth fault is latent: a part whose charset go-message does not decode
(anything but UTF-8 and US-ASCII) ends the part loop, so every part after it
is lost.

Faults 1, 2 and the latent one share a cause, and fault 3 is the same
cause at the send step: the rules that decide which part is the text
body, which is the HTML body and which is a file are our own, written case
by case, and were tested only against messages written by hand. The
fix is a process, not a fourth patch:

- The rules come from a published algorithm, RFC 8621 §4.1.4
  `parseStructure`, which Stalwart and Thunderbird's new protocol code use
  through the `mail-parser` crate.
- A corpus of real and malformed messages, most with expected results,
  gates every change.
- When parsing still goes wrong in production, the log says what the
  message looked like, without its content.

## Goals

- **Accept liberally.** The parser does its best to salvage something from
  every part, however malformed. It never refuses a message, and it never
  discards bytes it could show or store: what it cannot decode it keeps
  undecoded, and what it cannot classify it keeps as a file.
- Every leaf part of every message ends up as text, HTML or a file. No part
  is lost silently, and parsing never stops part-way.
- Text in any charset that `golang.org/x/text` knows is decoded to UTF-8.
- For the messages in the corpus that have expected results, the text-body,
  HTML-body and attachment lists match them, except for a short, reviewed
  list of deviations.
- A message with only a subject, or only a subject and files, is valid: it
  is stored and sent.
- The faults above are fixtures in the corpus.
- A message that parses badly in production leaves a log line that names
  its structure.

## Non-goals

- Building messages (`buildRFC5322`), IMAP FETCH, and the storage schema.
  `storedMessage` keeps its fields.
- The Postmark inbound webhook (`postmark.go` `ParseInbound`): Postmark sends
  JSON it has already split, not MIME.
- S/MIME, PGP, TNEF (`winmail.dat`) and calendar invitations beyond keeping
  their parts as files.
- Replacing go-message. It stays the reader; go-imap v2 and our message
  building use it too.

## Design

The parser becomes three steps, each a small unit with its own tests:

```
raw bytes ──readTree──▶ part tree (+ warnings) ──parseStructure──▶ text/html/attachment lists ──flatten──▶ storedMessage
```

`parseRFC5322(raw)` no longer returns an error; it always returns a
message, with the warnings beside it. Its four callers change with it:

```go
func parseRFC5322(raw []byte) (*storedMessage, []parseWarning)
```

The SMTP "message could not be parsed" (554) path goes away: a message that
reaches DATA is always stored or sent with whatever was salvaged.

### Step 1: `readTree` (new file `server/mimetree.go`)

`readTree(raw []byte) (*mimePart, []parseWarning)` reads the whole
tree with go-message's entity reader (`message.Read`, recursing through
`MultipartReader`), not `mail.Reader`, so containers, dispositions and
order are all kept.

```go
type mimePart struct {
	Type        string            // lower case, e.g. "text/html"; "text/plain" when missing
	Params      map[string]string // content-type parameters
	Disposition string            // "inline", "attachment" or ""
	Name        string            // filename (RFC 2231 and 2047 decoded) or content-type name
	ContentID   string
	Body        []byte            // transfer-decoded; text/* also charset-decoded to UTF-8
	Children    []*mimePart       // multipart/* only
	Index       int               // depth-first position, for logs and tests
}
```

The rule for every case below is the same: **keep the part, keep the most
useful bytes we can get, add a warning, and go on.** Nothing here returns an
error, and nothing ends the walk of the tree.

- `server` imports `github.com/emersion/go-message/charset`, so the common
  charsets decode.
- **Headers:**
  - **Header block cannot be read:** a top-level header block that cannot be
    read at all (for example binary garbage before the first blank line)
    makes the whole input one `text/plain` part. Its body is the raw bytes,
    charset-guessed as below.
  - **Bad header field:** a single malformed field is skipped; the others
    stay.
- **Content type:**
  - **Missing or unparseable `Content-Type`:** the part is `text/plain`, as
    RFC 2045 says. If the raw value starts with `multipart/` and a boundary
    can still be found in it, the part is read as that multipart.
  - **Bad parameters** (broken RFC 2231 continuations, unbalanced quotes):
    each parameter keeps its raw value when it cannot be decoded, so a
    filename survives as written.
- **Transfer encoding:**
  - **Unknown `Content-Transfer-Encoding`:** the body is kept as it is.
  - **Invalid base64 or quoted-printable:** the part is decoded up to the
    first bad byte, and the rest is kept raw (skipping whitespace and
    characters that cannot be base64, as lenient decoders do). If nothing
    decodes, the raw body is kept.
- **Charset:**
  - **Unknown, missing or wrong:** go-message returns the part together
    with an unknown-charset error, and the part is kept. Its body is used as
    is when it is valid UTF-8. For HTML, a `<meta charset>` in the body is
    tried next. Otherwise the body is decoded as Windows-1252, which maps
    every byte to some character.
  - **Text that still is not valid UTF-8** after decoding has its bad
    sequences replaced with U+FFFD, so the text can always be stored and
    shown.
- **Multipart structure:**
  - **Boundary missing, or never found in the body:** the body becomes one
    leaf. It is `text/plain` when it looks like text, and a file otherwise.
  - **Truncated multipart** (no closing delimiter, or the input ends inside
    a part): every part read so far is kept, including the last, partial
    one.
  - **Any other read error inside a multipart:** the parts read so far are
    kept, and the rest of that multipart's body, from the failing point,
    becomes one more leaf. Its siblings and parents go on.
  - **Preamble and epilogue:** dropped, as RFC 2046 says, unless the
    multipart has no parts at all. The preamble is then its text.
- **Other cases:**
  - **`message/rfc822` and other `message/*`:** a leaf, kept as bytes and not
    recursed into, as RFC 8621 says. It becomes an `.eml` file.
  - **Nesting:** limited to 50 levels. A deeper multipart becomes one file
    part with its raw bytes (against nesting bombs).
  - **Single-part messages:** the root is itself a leaf.
  - **Empty parts:** kept. They are in the tree, but a body list or the file
    list skips a part with no bytes.

### Step 2: `parseStructure` (new file `server/bodystructure.go`)

`parseStructure(root *mimePart) (text, html, attachments []*mimePart)` is
a direct port of the RFC 8621 §4.1.4 algorithm:

- `isInline`:
  - not `Content-Disposition: attachment`;
  - type `text/plain`, `text/html` or `image|audio|video/*`;
  - and first in its multipart, or (not in `multipart/related` and
    (inline media or no name)).
- In `multipart/alternative`: `text/plain` goes to the text body, `text/html`
  to the HTML body, and anything else is an attachment.
- Inside an alternative, a `text/plain` part ends the HTML list for that
  branch, and a `text/html` part ends the text list.
- Inline media is in both body lists, and is also an attachment when only
  one list takes it.
- At the end of an alternative, a branch that filled only one list copies its
  parts into the other.
- A single-part root is handled as `parts = [root]` in a `mixed` multipart.

RFC 8621 calls this algorithm "a starting point … likely to require
workarounds for malformed content". Each workaround we add:

- is justified by a named corpus message that fails without it;
- is written as its own small rule with that message named in its comment;
- is listed in the design section of the code (`bodystructure.go` header
  comment).

This lets us tell a workaround from a bug. Stalwart's issue #67
fixes (an alternative child with disposition attachment, a named HTML part
in an alternative, an unterminated alternative) are the first candidates;
the harness shows which ones we need.

### Step 3: `flatten` (in `server/bodystructure.go`)

`flatten` maps the three lists onto `storedMessage`, which holds one text
string, one HTML string and a file list:

- **TextBody:** the text-body parts in order, joined with `"\n"`.
  - `text/plain` is used as is.
  - `text/html` (there when a branch had only HTML) goes through
    `stripHTMLToText`.
  - Media parts are left out of the text.
- **HTMLBody:** the HTML-body parts in order, joined.
  - `text/html` is used as is. Each part's own `<html>`/`<body>` wrappers stay;
    `sanitizeEmailHTML` flattens them later.
  - `text/plain` becomes `<div style="white-space: pre-wrap">` with the text
    escaped.
  - A media part becomes `<img src="cid:…">` at its place in the order, unless
    an HTML part already references its Content-ID. A part with no Content-ID
    gets a generated one (`part-<index>@tinycld.invalid`). An image placed
    between two HTML parts then shows where the sender put it.
- If the HTML-body list holds no `text/html` part at all, HTMLBody stays
  empty. A plain-text message stays plain text, as today.
- **Attachments:** the RFC attachment list plus every media part of the body
  lists, without duplicates, in depth-first order.
  - Every file keeps its Content-ID, so `buildCIDMap` can resolve `cid:` links.
  - A part with no name is named from its type (`newInboundAttachment`, as
    today).
- **Text from HTML:** if TextBody is empty or only whitespace and HTMLBody is
  not, TextBody is `stripHTMLToText(HTMLBody)`, as today.

### Warnings and diagnostics

`parseWarning` is `{Part int; Kind string; Detail string}`. `Kind` is one of:
- `bad-header`
- `bad-content-type`
- `bad-header-param`
- `bad-encoding`
- `unknown-charset`
- `invalid-utf8`
- `bad-multipart`
- `truncated`
- `too-deep`

The callers log a warning line through `logging.ForPackage("mail")`:
- when there are warnings;
- when, after flattening, both bodies are empty but the tree has a `text/*`
  leaf that is not an attachment.

The line carries the structure and no content:

```
msg="message parsed with problems" user=… route=smtp-submission
structure="0 multipart/mixed; 1 text/html charset=windows-1252 →html; 2 application/pdf inline 48211B →file; 3 text/html charset=windows-1252 →html"
warnings="1 unknown-charset windows-1252 (decoded as Windows-1252)"
```

The structure lists, for each part: its index, type, charset, disposition,
size, and which list it went to. It never includes names, addresses or body
text. With this line, the third fault would have been found from the log,
without asking for the raw source.

### A message with no text

A message with only a subject, or a subject and files, is valid mail:
someone sends a file, or puts everything in the subject. It is stored with
empty bodies and sent with `TextBody = " "` when both bodies are empty.

Postmark refuses an empty body and accepts whitespace. Checked on
2026-11-04 with Postmark's test token (`POSTMARK_API_TEST`, validated but
not delivered):

| Body sent | Postmark |
|---|---|
| `TextBody: ""` (or no body) | ErrorCode 300, "Provide either 'TextBody' or 'HtmlBody'" |
| `TextBody: " "` | accepted |
| `TextBody: "\r\n"` | accepted |
| `HtmlBody: " "` | accepted |

The mailer (`core/server/mailer/postmark.go`) sends the fields as given and
trims nothing, so `" "` reaches Postmark as is. The send path
(`smtp_session.go`) sets it; the stored message keeps its empty bodies, so
the UI shows no body rather than a space.

## The corpus harness

### Layout

`server/testdata/corpus/<source>/` holds the messages of one source, and a
`SOURCE.md` with the origin URL, the commit it was taken from and the
license text.

| Source | From | Expected results | License |
|---|---|---|---|
| `stalwart/rfc`, `malformed`, `regressions`, `issues`, `thirdparty` | stalwartlabs/mail-parser `resources/eml` | yes (`.json`) | Apache-2.0 OR MIT; `thirdparty` is Dovecot's, MIT |
| `stalwart/legacy` | the Hunny Software samples in the same tree | yes | Hunny Software copyright; redistributed by stalwart and (with permission) zbateson; vendored with its COPYING note |
| `enmime` | jhillyerd/enmime `testdata/mail` | no | MIT |
| `mimekit` | jstedfast/MimeKit `UnitTests/TestData/messages` | no | MIT |
| `zbateson` | zbateson/mail-mime-parser `tests/_data/emails` (GitHub-issue messages; the Hunny ones duplicate `legacy`) | no | BSD-2 |
| `cpython` | python/cpython `Lib/test/test_email/data` | no | PSF |
| `thunderbird` | comm-central `mailnews/test/data` MIME torture test | no | MPL-2.0 (file-level) |
| `faults` | the faults above. Fault 3 is the real message, anonymized: names and addresses replaced, each attachment's bytes replaced by the same number of placeholder bytes (the structure, headers, boundaries, encodings and sizes stay) | yes (ours) | ours |
| `clients` | real client messages (below) | yes (ours) | ours |

A script, `server/testdata/corpus/fetch.sh`, takes the external sources at
pinned commits, so a refresh is a reviewed diff and not a hand copy.

### The tests (`server/corpus_test.go`)

1. **`TestCorpus_LosesNothing`** runs on every message of every source:
   - every leaf of the tree with bytes is in the text, HTML or attachment
     list;
   - no leaf that had bytes in the raw message has an empty body;
   - whenever the tree has a `text/*` leaf that is not an attachment, the
     text or the HTML body is not empty;
   - the result is the same on a second run.
2. **`TestCorpus_MatchesExpected`** runs on the stalwart sets:
   - **Expected side:** for message 0, the expected `text_body`, `html_body`
     and `attachments` part lists, each part as (type, decoded content) from
     the JSON's `text` or `binary` field.
   - **Our side:** the three lists from `readTree` + `parseStructure`, before
     flattening, as (type, decoded body).
   - Content is compared with CRLF changed to LF on both sides.
   - A mismatch fails the test unless `known_deviations.txt` lists that
     message with a reason. A listed message that now matches also fails the
     test, so the list can only shrink.
3. **`TestCorpus_Flatten`** runs on `faults` and `clients`. Each message has
   an `NAME.expect.json` with the `storedMessage` facts that matter:
   - text and HTML snippets that must appear;
   - the attachments by name, type and size;
   - that no body is empty.
4. **`FuzzParseRFC5322`** is seeded with every corpus message. It checks:
   - no panic, and always a message back;
   - the LosesNothing properties;
   - that each input is parsed within one second.

   It runs as a normal test over the seeds in CI. Longer fuzz runs are local.

The three steps also get unit tests of their own: `readTree` errors and
warnings, the RFC algorithm branch by branch, and the flatten mapping.

### Real-client messages

No public corpus has mail from current clients. We build our own:

- **Cases:**
  1. plain text;
  2. rich text;
  3. an inline image;
  4. a PDF placed inside the text;
  5. non-ASCII text (accents, emoji, a right-to-left line);
  6. a reply with a quote;
  7. a forward as attachment;
  8. a message with a file and no text.
- **Clients:**
  - Apple Mail on macOS and iOS;
  - Outlook for Windows and Outlook on the web;
  - Gmail web and the Gmail app;
  - Thunderbird.
- **Capture:** send each case to a test mailbox and save the raw source with
  the client's or webmail's "show original" or "raw source". Run
  `testdata/corpus/clients/anonymize.go` over it. It replaces addresses,
  names, Message-IDs and `Received` hosts and keeps the structure byte for
  byte.
- **Expected results:** written by hand from what the sending client showed.
- This needs a person with those clients. The harness and the other sources
  do not wait for it. The set grows as clients are captured.

## Rollout

1. Land the harness and the corpus first, with the current parser, and
   record its failures. This is the baseline. It shows how much the new
   parser fixes, and that it fixes the live fault.
2. Land `readTree`, `parseStructure` and `flatten` behind the same
   `parseRFC5322` signature.
3. Land the diagnostics.
4. Release mail as a patch. Move tinycld.com to it with
   `pnpm run hosting -- expedite mail@<version>`.

## Testing

- Corpus: the four tests above, on about 334 messages, about 0.7 MB in the
  repo (fault 3 is larger, about 5 MB, unless its attachments are shortened;
  see the anonymizer).
- Unit: `readTree`, `parseStructure` (a test per RFC branch), `flatten`.
- The callers' existing tests (`imap_rfc5322_test.go`, `message_files_test.go`,
  SMTP and IMAP tests) keep passing, with the new warnings return value.

## Open questions

1. **Who captures the real-client messages, and from which accounts?**
2. **Fault 3's size:** keep its 4.9 MB attachment at full length (it tests
   a large base64 part), or shorten it to a few KB in the fixture and cover
   large parts with a generated test instead?
