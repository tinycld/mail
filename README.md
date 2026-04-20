# @tinycld/mail

Full-featured email for your organization — threaded conversations, rich-text composer, attachments, delivery tracking, a privacy-preserving image proxy, and a native IMAP + SMTP server so any desktop mail client just works.

Part of [TinyCld](https://tinycld.org/) — the open-source, self-hosted Google Workspace alternative.

## Features

- **Threaded inbox.** Conversations are grouped server-side and updated live. Per-thread read/starred/folder state is scoped to each user-org so shared mailboxes don't step on each other.
- **Personal & shared mailboxes.** A mailbox can be owned by one user or shared across an org with role-based access.
- **Rich-text composer.** Web and native editors, with reply/forward threading, attachments, and inline replies.
- **Folders.** Inbox, Sent, Drafts, Trash, Spam, Archive — per-user state, not per-mailbox.
- **Delivery tracking.** Each outgoing message carries a `delivery_status` (`sending` → `sent` → `delivered` | `bounced` | `spam_complaint`) updated from bounce webhooks.
- **Advanced search.** Full-text search across subjects, snippets, senders, and bodies via the server's search endpoint.
- **Private-by-default image proxy.** External images in HTML mail are rewritten to proxy through your server with a scoped token — senders never see your IP, user-agent, or read receipts. No tracking pixels reach your browser.
- **HTML sanitization.** Every inbound HTML body is sanitized server-side before it touches a client.
- **Custom domains.** Verify your own domain end-to-end (MX, SPF, DKIM, return-path, inbound routing) from the Provider settings screen.
- **Postmark integration.** Outbound + inbound routing through Postmark for deliverability, with automatic domain verification polling.
- **IMAP server.** Native `imap_server.go` supports IDLE, folder mapping, UID validity, and RFC 5322 message fetching. Apple Mail, Thunderbird, mutt — point them at port 993 and go.
- **SMTP submission.** Native SMTP endpoint on port 465 so any client can send through TinyCld.
- **Real-time updates.** New messages, read state, and thread moves appear across all sessions instantly.
- **Contact suggestions.** Composer auto-completes recipients from `@tinycld/contacts` when the package is installed.

## Protocols

| Protocol | RFC       | Port | Purpose                                |
|----------|-----------|------|----------------------------------------|
| IMAP     | RFC 9051  | 993  | Read email from any mail client        |
| SMTP     | RFC 5321  | 465  | Send email through any mail client     |

## Relationship to core

`@tinycld/mail` is a feature package for `@tinycld/core` — the [TinyCld](https://tinycld.org/) app shell that provides auth, routing, storage, and UI primitives. Core ships with **no** feature packages; install this one to add a Mail app.

This package contributes:

- **Screens** — org-scoped routes at `/a/<org>/mail` (inbox list, thread detail).
- **Settings panels** — Provider configuration (Postmark credentials, domain verification) and Mailboxes management.
- **Nav entry** — sidebar icon with keyboard shortcut `t m` / `m`.
- **Collections** — `mail_domains`, `mail_mailboxes`, `mail_mailbox_members`, `mail_threads`, `mail_messages`, `mail_thread_state`, `mail_imap_mailbox_state`.
- **Migrations** — schema and indexes under `pb-migrations/`.
- **Go server module** — IMAP server, SMTP server, sanitizer, image proxy, bounce webhooks, inbound webhook, draft/send endpoints, domain verification ticker, and search — all wired into core's PocketBase binary.

The package depends on core at runtime (React, pbtsdb, `~/lib/*`). Core has no knowledge of this package at compile time — everything is discovered at generator time from `tinycld.packages.ts`.

## Installation

From inside your `core/` checkout:

```sh
bun run packages:install <this-repo-git-url>
```

That clones the repo next to core, symlinks it into `core/packages/@tinycld/mail`, appends the package name to `tinycld.packages.ts`, and runs the generator to wire up routes, collections, migrations, settings panels, and Go server extensions.

To remove:

```sh
bun run packages:unlink @tinycld/mail
```

## Development

This package is not run standalone — it only makes sense inside a `core/` checkout.

```sh
cd ../core
bun run dev              # expo + pocketbase (with IMAP/SMTP servers) with mail linked
bun run test:unit        # includes this package's sanitizer, image-proxy, and domain-verify tests
bun run checks           # biome + tsc across core + linked packages
```

**Do not** run `bun install` inside this directory. Peer dependencies resolve through core's `node_modules/`; installing here creates duplicate copies of `react`, `react-native`, etc. and breaks TypeScript.

## License

See the root TinyCld repository for licensing.
