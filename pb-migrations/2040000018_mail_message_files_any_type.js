/// <reference path="../../../server/pb_data/types.d.ts" />

// PocketBase checks a file field's mimeTypes by sniffing the file's first
// bytes, not by its name. The server writes both of these fields itself, and
// the sniffer rejects valid content:
//   - body_html: sanitized HTML that starts with text, <span>, <blockquote> or
//     <img> sniffs as text/plain, so inbound mail failed to store.
//   - raw_headers: headers that start with Received:, Return-Path: or From:
//     sniff as message/rfc822, so the headers were silently dropped.
// The client reads both as text, so the type restriction protected nothing.
migrate(
    app => {
        const messages = app.findCollectionByNameOrId('mail_messages')
        messages.fields.getByName('body_html').mimeTypes = []
        messages.fields.getByName('raw_headers').mimeTypes = []
        app.save(messages)
    },
    app => {
        const messages = app.findCollectionByNameOrId('mail_messages')
        messages.fields.getByName('body_html').mimeTypes = ['text/html']
        messages.fields.getByName('raw_headers').mimeTypes = ['text/plain', 'application/octet-stream']
        app.save(messages)
    }
)
