---
title: Setting up a mail provider
summary: Choose Postmark or run your own SMTP — either way mail flows through your provider
tags: [provider, postmark, smtp, setup, settings]
order: 100
---

## Who can do this

The mail provider is deployment-wide infrastructure — all mail goes through one provider account. A **super admin** (or the PocketBase superuser) configures it from the Setup dashboard's **Settings** tab (`/setup`). Mail **domains**, by contrast, are managed in-app at **Settings → Mail → Provider** by organization **owners** and **admins**.

## Picking a provider

Two providers ship today:

- **Postmark** — managed SaaS. Sign up, paste two API tokens, point your DNS at Postmark's hosts. Best for most people.
- **Self-hosted SMTP** — TinyCld delivers mail directly via SMTP and accepts inbound either as an MX target or by polling an IMAP account. No third-party account required, but you take on the deliverability work yourself (rDNS, SPF, DKIM, monitoring).

You can switch between them later without losing stored mail.

## Postmark

A **Postmark** account ([postmarkapp.com](https://postmarkapp.com)) with at least one server. Specifically:

- **Postmark account token** — found at **Account → API Tokens**. Used for domain operations (creating, verifying, listing).
- **Postmark server token** — found inside a specific server, at **Server → API Tokens → Server API token**. Used for sending and inbound.

Postmark has a free tier suitable for testing; production use needs a paid plan.

### Connecting

1. Open the Setup dashboard's **Settings** tab and find the **Mail Provider** panel.
2. Pick **Postmark** as the provider.
3. Paste your **server token** and **account token**.
4. Click **Save**.

That's the whole setup. Once saved, outbound mail submitted from any composer routes through this Postmark server, and inbound mail routed to your verified domains lands here.

## Self-hosted SMTP

This provider does the SMTP work itself. Outbound mail goes straight to the recipient's mail server via MX lookup; inbound mail arrives either at TinyCld's own SMTP listener (port 25 — TinyCld is your MX target) or via a background IMAP fetcher that polls an existing mailbox.

### What you need

- A host that can make **outbound TCP 25**. Most cloud providers (AWS, GCP, Azure, Hetzner) block port 25 by default for new accounts — you may need to file a support ticket to unblock it, switch to a relay-style smarthost, or use a VPS provider that allows port 25 out of the box.
- For inbound MX mode: the same host must be able to **accept inbound TCP 25** from anywhere on the internet.
- Working **reverse DNS** (PTR record) for the host's IP — receivers spam-folder mail from hosts without rDNS.
- Published **SPF / DKIM / DMARC** records for every sending domain (see below). Without DKIM, expect every major provider to spam-folder you.

### Connecting

1. Open the Setup dashboard's **Settings** tab and find the **Mail Provider** panel.
2. Pick **Self-hosted SMTP**.
3. Fill in:
   - **Public hostname** — the host that mail receivers will see (matches your rDNS). This is also the MX target operators will publish if they use inbound MX mode.
   - **DKIM selector** — defaults to `tinycld`. Used to look up `<selector>._domainkey.<domain>` during DNS verification.
   - **Inbound mode** — see below.
4. Click **Save**.

### Inbound mode

Three choices:

- **None (outbound only)** — TinyCld sends mail but doesn't try to receive. Useful if you're using TinyCld as a sending relay and reading mail elsewhere.
- **Built-in SMTP listener** — TinyCld listens on port 25 and accepts mail addressed to any verified domain. The MX records you publish should point at the **Public hostname**. Requires `MAIL_INBOUND_SMTP_ENABLED=true` set on the running server process (an operator-level guard; the listener won't bind otherwise).
- **Poll IMAP mailbox** — TinyCld polls an existing IMAP account periodically and ingests anything it finds in the configured mailbox. Useful when you already have a mail server you'd rather keep, or when port 25 inbound isn't available.

### IMAP polling

When inbound mode is **Poll IMAP mailbox**, the following appear:

- **IMAP host** / **port** — the server to poll. Port `0` picks the default (`143` plain, `993` TLS).
- **Username** / **password** — IMAP credentials. Stored like the other deployment settings, with the password masked.
- **Mailbox** — defaults to `INBOX`.
- **Poll interval (seconds)** — defaults to `60`. Lower = fresher mail at the cost of more IMAP traffic.
- **Use TLS** — turn off only when polling localhost or a host with broken TLS.

The fetcher runs a single poll loop for the deployment, marks each fetched message as `\Seen` after a successful store, and re-runs only on the unseen set — so a message is ingested exactly once even if it restarts mid-tick.

### Bounce model (synchronous SMTP errors)

Self-hosted SMTP doesn't use a bounce webhook. Instead, the SMTP conversation itself reports failures: if a recipient's server returns a permanent **5xx** during RCPT TO, TinyCld marks the message as `bounced` with the SMTP code as the bounce reason. Async DSN messages (bounce notifications that arrive later as inbound mail) are not parsed — they land in the configured inbox alongside regular mail.

### DKIM signing

DKIM **signing** of outbound mail is not yet implemented for self-hosted SMTP. The provider verifies you have a DKIM record in DNS (so the verification UI is green), but TinyCld won't actually sign messages. Most modern receivers spam-folder unsigned mail. If deliverability matters, run a smarthost (Postfix / opensmtpd) that signs outbound traffic, point your DNS at it, and use Postmark or another managed provider for now.

## Where credentials are stored

Provider settings live in the deployment-wide `system_settings` store (keys like `mail.provider`, `mail.postmark_server_token`), readable and writable only by super admins and the PocketBase superuser. Secrets (tokens, IMAP passwords) are masked in the settings UI.

The IMAP fetcher reconciles automatically whenever a setting changes, so credential rotation takes effect without a restart. There is no environment-variable fallback — the Settings tab is the one place providers are configured. (The only env switch is the operator-level `MAIL_INBOUND_SMTP_ENABLED` guard for the built-in listener.)

## Verifying it works

After saving credentials:

1. Go to **Settings → Mail → Provider** and add a domain (see [Custom domains](help://mail:custom-domains)).
2. Once the domain shows green checkmarks for MX / SPF / DKIM / Return-Path, send yourself a test message.
3. If the message goes through, you're set. If not, the error in the Sent folder's status indicator will point you at the failing piece.

## Common errors

- **"Provider not configured"** — you haven't saved a server token (Postmark) or selected a provider. Outbound is blocked.
- **"401 Unauthorized" from Postmark** — token is wrong or revoked. Regenerate in Postmark and re-save.
- **"403 Forbidden" from Postmark** — token is valid but doesn't have the permissions you need (e.g. an account token where the server token is required). Double-check which is which.
- **"connection refused" on port 25** (SMTP provider) — your cloud provider is blocking outbound 25. Open a ticket or switch hosting.
- **Mail goes to spam** (SMTP provider) — most often missing DKIM signing. See the DKIM section above.

## See also

- [Custom domains](help://mail:custom-domains)
- [Delivery tracking](help://mail:delivery-tracking)
- [Connecting an SMTP client](help://mail:smtp)
