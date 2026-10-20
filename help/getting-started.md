---
title: Getting started with Mail
summary: A tour of inboxes, folders, and the inbox view
tags: [intro, basics, tour]
order: 10
---

## What Mail is

A full email client backed by a mail provider like Postmark for actual delivery. You can:

- Read and write email in the web / iPad UI.
- Connect any standard mail client (Outlook, Apple Mail, Gmail, Thunderbird) via [IMAP](help://mail:imap) and [SMTP](help://mail:smtp).
- Use your own custom domain — set up MX, SPF, DKIM, and inbound routing once and Mail handles the rest.

## Inboxes vs folders

An **inbox** is a complete email account — an address like `you@example.com` plus its history. You can have:

- **A personal inbox** — automatically created the moment your account is created. See [Your email addresses](help://mail:addresses).
- **Shared inboxes** — like `support@` or `sales@`. Multiple people can read and send from one inbox, with `owner` and `member` roles.

Inside each inbox there are **folders** that group threads: Inbox, Starred, Sent, Drafts, All Mail, Spam, Trash, Archive. Folders are per-user — what *you* mark as starred or move to Archive doesn't affect other members of a shared inbox.

## The sidebar

- **Compose** button — top of the sidebar, opens the compose window from anywhere.
- **All Inboxes** — only appears when you have 2 or more inboxes; shows everything new across all of them.
- **One section per inbox** — expanded by default, with the 8 folders inside.
- **Labels** — colored tags you can apply to threads. Same label system used by other packages — see [Labels](help://mail:labels).

## A thread vs a message

Mail groups related messages into **threads** (conversations). Replying to a thread keeps everything in one place; viewing a thread shows every message in order. Threading uses RFC 5322 `In-Reply-To` and `References` headers, with a subject-line fallback when those are missing.

## Where to go next

- [Your email addresses](help://mail:addresses)
- [Shared inboxes](help://mail:mailboxes)
- [Composing and sending](help://mail:composing)
- [Reading threads](help://mail:reading-threads)
- [Folders](help://mail:folders)
- [Labels](help://mail:labels)
- [Search](help://mail:search)
- [Setting up a custom domain](help://mail:custom-domains)
- [Connecting Outlook, Apple Mail, Gmail or Thunderbird (IMAP)](help://mail:imap)
