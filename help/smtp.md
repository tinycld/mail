---
title: Connecting an SMTP client
summary: Send email from Outlook, Apple Mail, Gmail, Thunderbird, or any SMTP-capable client
tags: [smtp, send, submission, outlook, apple-mail, gmail, thunderbird]
order: 130
---

## What SMTP gives you

SMTP submission lets your desktop or phone mail client *send* through TinyCld — the outbound counterpart to [IMAP](help://mail:imap) for reading. Once configured, hitting Send in Outlook, Apple Mail, Gmail or Thunderbird routes through TinyCld → your mail provider → the recipient.

This is **not** the same as the SMTP your mail provider uses to deliver mail across the internet. This is the *submission* endpoint — you authenticate as a TinyCld user, TinyCld validates the sender address is one you have access to, and then forwards the message through your configured provider with all the right SPF / DKIM signing.

> **Note**: this page is about connecting *your client* to TinyCld over SMTP. It is unrelated to the **self-hosted SMTP provider** option in [Provider setup](help://mail:provider-setup), which controls how TinyCld delivers and receives mail on the wire.

## The quickest way

Open **Settings → Account → Mail apps**. The page shows your exact settings. On a Mac, iPhone or iPad, **Download the profile** puts them in Mail; your device then asks for your password. A downloaded profile does not install by itself. On an iPhone or iPad, open Settings and tap **Profile Downloaded**, then **Install**. On a Mac, open System Settings and search for **Profiles**. The device says the profile is "Unverified". This is normal. To set up your phone, scan the QR code on the page with its camera.

Most mail apps also find the settings by themselves when you type your email address and password, if your admin published the [DNS records for mail apps](help://mail:custom-domains).

## Connection settings

| Setting | Value |
|---|---|
| **Server** | `{{mail-host}}` |
| **Port** | **465** |
| **Encryption** | SSL / TLS (implicit) |
| **Username** | your full email address |
| **Password** | your TinyCld password, or an app password |
| **Authentication** | Normal password |

> **The server is `{{mail-host}}`.** Type it exactly. Your mail domain or a `mail.` name works only when your admin published the DNS records for mail apps and the server has a certificate for that name. Otherwise the mail app shows a certificate error or cannot connect.

The same credentials as [IMAP](help://mail:imap): your regular TinyCld login.

> **Two-step verification.** If two-step verification is on, or your organization requires it, your sign-in password does not work in mail apps. Create an app password in **Settings → Security → App passwords** and use it. See [App passwords](help://core:security-app-passwords).

## Why port 465 (not 587)

TinyCld's SMTP server uses **implicit TLS on port 465** in production. STARTTLS on port 587 isn't exposed — implicit TLS is simpler, more secure, and has fewer ways to misconfigure (no "downgrade to plain" failure mode).

Most clients support this — just pick **SSL / TLS** (not STARTTLS) when prompted, and port 465.

## Connecting Apple Mail (macOS)

This is usually done as part of [IMAP setup](help://mail:imap) — the same account holds both incoming and outgoing config. If you need to add outgoing separately:

1. **Mail → Settings → Accounts**, pick the TinyCld account.
2. Click the **Server Settings** tab.
3. Under **Outgoing Mail Server (SMTP)**:
   - Host Name: `{{mail-host}}`.
   - User Name: your full email address.
   - Password: your TinyCld password.
   - Use TLS/SSL: **Yes**.
   - Port: **465**.
   - Authentication: **Password**.
4. Click **Save**.

## Connecting Thunderbird

1. **Account Settings → Outgoing Server (SMTP) → Add**.
2. Description: anything (e.g. "TinyCld").
3. Server Name: `{{mail-host}}`.
4. Port: **465**.
5. Connection security: **SSL/TLS**.
6. Authentication method: **Normal password**.
7. User Name: your full email address.
8. Click **OK**.

Then in your account's Server Settings, set this SMTP server as the outgoing server for the account.

## Connecting Outlook and the Gmail app

Outlook (Windows and macOS) and the Gmail app on Android ask for the outgoing server when you add the account. The steps are in [IMAP](help://mail:imap). Use server `{{mail-host}}`, port **465** and **SSL/TLS**, with the same user name and password.

## Which address you can send from

When your SMTP client submits a message, TinyCld checks the sender address your mail app gives (the envelope sender, usually the same as the account's email address). The address must be:

- The primary address of an inbox you're a member of, OR
- One of the other [addresses](help://mail:addresses) of an inbox you're a member of.

A group address cannot be a sender. If the address is not known, the server rejects with `550 Sender address not recognized`. If it belongs to an inbox you are not a member of, the server rejects with `550 Not authorized to send from this address`.

TinyCld then writes the message's `From:` header from that address and the inbox's display name, so the header always matches the address you sent from.

## Size limits

Outbound messages are capped at **25 MB total** (including attachments). Larger messages are rejected at the SMTP layer with `552`. For large files, send a link to a [Drive](help://drive:files) share instead.

## Why messages might be rejected

- **Connects, then immediately disconnects ("service unavailable")** — the server name in your client is wrong. It must be `{{mail-host}}`. Another name works only when your admin published the DNS records for mail apps and the server has a certificate for that name.
- **"Relaying denied. To send mail, log in on port 465."** — your mail app is sending through port 25, which only receives mail. Set the outgoing server port to **465**, turn on **SSL/TLS**, and turn on password authentication. In Apple Mail, clear **Automatically manage connection settings** first.
- **Auth failed** — wrong email address or password. Same as IMAP: use your full email address. With two-step verification on, the password must be an [app password](help://core:security-app-passwords).
- **Auth fails with `Your mail access is read-only; sending is not permitted`** — your account has read-only access to Mail on this server. You can still read over [IMAP](help://mail:imap); sending needs an administrator to grant full access.
- **Sender address not recognized / Not authorized to send from this address** — see above; the sender address has to be one of the addresses of an inbox you belong to.
- **Domain not verified** — the sending domain has to be fully verified (MX, SPF, DKIM, Return-Path). See [Custom domains](help://mail:custom-domains).
- **Provider not configured** — the deployment has no mail provider yet. Configuring one is an administrator task; where a service provider runs this deployment, it is theirs to fix.
- **Message size too large** — 25 MB max.
- **Storage limit exceeded** — storing your Sent copy would push this server over its storage limit. Free space by deleting large threads, or ask an administrator to raise the limit.

## See also

- [IMAP](help://mail:imap) — the receive side
- [Your email addresses](help://mail:addresses)
- [Shared inboxes](help://mail:mailboxes)
- [Custom domains](help://mail:custom-domains)
