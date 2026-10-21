---
title: Connecting an IMAP client
summary: Read your mail from Outlook, Apple Mail, Gmail, Thunderbird, or any IMAP-compatible client
tags: [imap, outlook, apple-mail, gmail, thunderbird, android, mobile]
order: 120
---

## What IMAP gives you

IMAP lets you read your TinyCld mail from any desktop or phone mail client. Once configured:

- **Folders sync** — Inbox, Sent, Drafts, Spam, Trash, Archive show up in your client.
- **Unread state syncs** — marking a message read in your client updates TinyCld and vice versa.
- **Multi-inbox support** — every inbox you're a member of (personal + shared) appears in your client.
- **IDLE** — modern IMAP clients use the IDLE extension to get push-style updates. TinyCld supports it, so new messages appear within a second of arriving.

IMAP is **read + state**, not send. To also send from your client, set up [SMTP](help://mail:smtp).

## The quickest way

Open **Settings → Account → Mail apps**. The page shows your exact settings. On a Mac, iPhone or iPad, **Download the profile** puts them in Mail; your device then asks for your password. A downloaded profile does not install by itself. On an iPhone or iPad, open Settings and tap **Profile Downloaded**, then **Install**. On a Mac, open System Settings and search for **Profiles**. The device says the profile is "Unverified". This is normal. To set up your phone, scan the QR code on the page with its camera.

Most mail apps also find the settings by themselves when you type your email address and password, if your admin published the [DNS records for mail apps](help://mail:custom-domains).

## Connection settings

| Setting | Value |
|---|---|
| **Server** | `{{mail-host}}` |
| **Port** | **993** |
| **Encryption** | SSL / TLS (implicit) |
| **Username** | your full email address on this server (not your recovery email) |
| **Password** | your TinyCld password, or an app password |
| **Authentication** | Normal password |

> **The server is `{{mail-host}}`.** Type it exactly. Your mail domain or a `mail.` name works only when your admin published the DNS records for mail apps and the server has a certificate for that name. Otherwise the mail app shows a certificate error or cannot connect.

There is no separate "IMAP password". Use your regular TinyCld login.

> **Two-step verification.** If two-step verification is on, or your organization requires it, your sign-in password does not work in mail apps. Create an app password in **Settings → Security → App passwords** and use it. See [App passwords](help://core:security-app-passwords).

## Connecting Apple Mail (macOS)

1. **Mail → Settings → Accounts → +**.
2. Choose **Other Mail Account…**, click **Continue**.
3. Enter your name, email address, and TinyCld password. Click **Sign In**.
4. Apple Mail will probably fail auto-config — that's expected; the manual page appears.
5. **Account Type**: IMAP.
6. **Incoming Mail Server**: `{{mail-host}}`.
7. **User Name**: your full email address on this server.
8. **Password**: your TinyCld password.
9. **Outgoing Mail Server**: `{{mail-host}}`, the same user name and password. Port 465 with SSL/TLS.
10. Click **Sign In**.

The mail account appears in the sidebar. Each inbox you have access to shows up as a separate account folder.

## Connecting Apple Mail (iOS / iPadOS)

1. **Settings → Mail → Accounts → Add Account → Other → Add Mail Account**.
2. Fill in your name, email, password.
3. Tap **Next** — iOS will try to auto-config and fail; the manual screen appears.
4. Pick **IMAP**.
5. Incoming Mail Server — Host Name: `{{mail-host}}`, User Name: your full email address on this server, Password: your password.
6. Outgoing Mail Server — same hostname, port 465. (Set up [SMTP](help://mail:smtp) to send.)
7. Tap **Save**.

## Connecting Thunderbird

1. **File → New → Existing Mail Account**.
2. Enter your name, email, password. Click **Continue**.
3. Thunderbird auto-discovers; if it doesn't, click **Configure manually**.
4. **Incoming**: Protocol IMAP, Hostname `{{mail-host}}`, Port 993, SSL/TLS, Authentication Normal password.
5. **Outgoing**: SMTP, same hostname, Port 465, SSL/TLS — see [SMTP](help://mail:smtp).
6. Click **Done**.

## Connecting Outlook (Windows)

In the new Outlook:

1. **Settings → Accounts → Email accounts → Add account**.
2. Enter your email address and click **Continue**.
3. If Outlook asks for the account type, choose **IMAP**.
4. Enter your TinyCld password. Then open the advanced settings.
5. **IMAP incoming server**: `{{mail-host}}`, port **993**, secure connection type **SSL/TLS**.
6. **SMTP outgoing server**: `{{mail-host}}`, port **465**, secure connection type **SSL/TLS**. Use the same user name and password.
7. Click **Continue**.

In classic Outlook:

1. **File → Add Account**.
2. Enter your email address. Open **Advanced options**, select **Let me set up my account manually**, and click **Connect**.
3. Choose **IMAP**.
4. **Incoming mail**: server `{{mail-host}}`, port **993**, encryption method **SSL/TLS**.
5. **Outgoing mail**: server `{{mail-host}}`, port **465**, encryption method **SSL/TLS**.
6. Click **Next**, enter your TinyCld password, and click **Connect**.

## Connecting Outlook (macOS)

1. **Outlook → Settings → Accounts → + → New Account**.
2. Enter your email address and click **Continue**.
3. If Outlook asks for the provider, choose **IMAP/POP**.
4. **Type**: IMAP. **User name**: your full email address on this server. **Password**: your TinyCld password.
5. **Incoming server**: `{{mail-host}}`, port **993**, **Use SSL to connect** on.
6. **Outgoing server**: `{{mail-host}}`, port **465**, **Use SSL to connect** on.
7. Click **Add Account**.

## Connecting the Gmail app (Android)

1. In the Gmail app, open the menu, then **Settings → Add account → Other**.
2. Enter your email address and tap **Manual setup**.
3. Choose **Personal (IMAP)** and enter your TinyCld password.
4. **Incoming server settings**: server `{{mail-host}}`, port **993**, security type **SSL/TLS**.
5. **Outgoing server settings**: keep **Require sign-in** on. Server `{{mail-host}}`, port **465**, security type **SSL/TLS**.
6. Tap **Next** to finish.

## Folder names

In every IMAP client you'll see these folder names per inbox (case may vary by client):

- **INBOX**
- **Sent** (flagged `\Sent`): only the messages you sent. The messages you replied to stay in their own folder.
- **Drafts** (flagged `\Drafts`)
- **Trash** (flagged `\Trash`)
- **Spam** (flagged `\Junk`)
- **Archive** (flagged `\Archive`)

If you turn on **Show a folder for each address** in **Settings → Account → Email**, your personal inbox also has an **Addresses** folder. It holds one folder for each address, for example **Addresses/you@example.com**. Each folder shows the messages in your INBOX that use that address. You cannot move, copy, or save messages into these folders, and you cannot rename or delete them. To change which folders show, use the switches in **Settings → Account → Email**. See [Your email addresses](help://mail:addresses).

When you have multiple inboxes, IMAP namespaces them — typically as separate sub-folders or as separate accounts (depending on your client). Each inbox's folders are independent.

## Troubleshooting

- **Connects, then immediately disconnects ("service unavailable" / BYE)** — the server name in your client is wrong. It must be `{{mail-host}}`. Another name works only when your admin published the DNS records for mail apps and the server has a certificate for that name.
- **Auth failed** — use your full email address on this server (an address of your own inbox, not of a shared inbox, and not your recovery email) and the password you sign in with. With two-step verification on, the password must be an [app password](help://core:security-app-passwords).
- **Connection refused / timeout** — verify your TinyCld instance is reachable on port 993. Try `openssl s_client -connect {{mail-host}}:993 -servername {{mail-host}}` from a terminal to test.
- **TLS certificate error** — Mail's IMAP server uses the TLS certificate of `{{mail-host}}`. Check that the server name in your client is exactly `{{mail-host}}`.
- **Flagging, moving, or deleting fails with `Your mail access is read-only`** — your account has read-only access to Mail on this server. Reading and syncing still work, but any change your client tries to make (mark read, star, move, delete, save a draft) is refused. Ask an administrator to grant full access if you need to make changes.
- **Folders don't sync** — IMAP IDLE should push new messages within a second, but some clients only check every 5–15 minutes by default. Look for a "check for new mail every N minutes" setting.

## See also

- [SMTP](help://mail:smtp)
- [Your email addresses](help://mail:addresses)
- [Shared inboxes](help://mail:mailboxes)
