---
title: Custom domains
summary: Adding your own domain so you can send and receive as you@yourdomain.com
tags: [domain, dns, mx, spf, dkim, postmark]
order: 110
---

## Why custom domains

Out of the box, Mail can send through whatever address your provider gives you. To actually use your own domain — `you@yourcompany.com` — DNS needs to be set up correctly: MX records to receive, SPF / DKIM to send, return-path for bounces, and Postmark needs to know about the domain. Mail walks you through all of this.

## Adding a domain

1. **Settings → Mail → Provider**.
2. Scroll to **Domains** and click **+ Add domain**.
3. Enter the bare domain (e.g. `example.com`, not `@example.com` or `https://example.com`).
4. Click **Add**.

The domain is created in TinyCld, and Postmark is asked to create a matching server-side domain record. Once that finishes, the row shows a checklist of verification steps.

## The verification checklist

Each domain has four checks, all four green is required for full send + receive. The exact targets depend on which [provider](help://mail:provider-setup) your org uses:

- **Inbound MX** — DNS MX records pointing at your provider's inbound host.
  - **Postmark**: `inbound.postmarkapp.com`.
  - **Self-hosted SMTP, "Built-in listener" inbound mode**: your TinyCld host's **Public hostname**.
  - **Self-hosted SMTP, "Poll IMAP" inbound mode**: not checked — mail arrives via IMAP polling instead of MX.
- **Provider** — for Postmark, the Postmark API confirms your domain's inbound forwarding is configured. For self-hosted SMTP, this confirms the provider's public hostname is set.
- **SPF** — DNS TXT record authorizing the sending host. Required so recipient servers don't mark your mail as spam.
- **DKIM** — DNS CNAME / TXT records for cryptographic signing of outgoing mail. Required for deliverability. Postmark generates the keys; self-hosted SMTP expects a record at `<selector>._domainkey.<domain>` where the selector comes from your provider settings (defaults to `tinycld`).
- **Return-Path** — DNS CNAME for bounce handling. Postmark provides the target; for self-hosted SMTP we use the presence of a DMARC record at `_dmarc.<domain>` as the indicator.

For each one, click **View setup instructions** to see the exact DNS records to add. Most DNS providers (Cloudflare, AWS Route 53, GoDaddy) let you paste these directly.

## Verifying

After adding DNS records, click **Verify now** in the domain row. Mail re-runs every check. DNS propagation can take a few minutes (occasionally hours), so it's OK to come back later.

Mail also re-verifies any not-yet-fully-verified domain automatically once per hour in the background. So if your DNS records are correct but propagating slowly, the domain will eventually flip to green on its own.

## What "fully verified" enables

- **Send** — outbound from any address on this domain works.
- **Receive** — inbound mail to any address on this domain delivered to a matching [mailbox or alias](help://mail:mailboxes) appears in that mailbox's Inbox.
- **Bounce tracking** — delivery callbacks update the `delivery_status` of sent messages. See [Delivery tracking](help://mail:delivery-tracking).

## Inbound routing

When a message arrives at `someone@yourdomain.com`, what happens next depends on which provider your org uses.

**Postmark**:

1. Postmark receives the message (because of your MX records).
2. Postmark POSTs the parsed JSON to your TinyCld instance at `/api/mail/inbound/{webhook_secret}`.
3. TinyCld looks up the recipient against `mail_mailboxes.address` and `mail_mailbox_aliases.address`, scoped to this domain.
4. If a match is found, the message is delivered to that mailbox's Inbox and replicated to every member.
5. If no match is found, TinyCld returns a 403 and Postmark generates a bounce back to the sender.

The webhook secret is a per-domain random 32-character hex string, auto-generated when the domain is created. URLs containing this secret are visible to org admins at **Settings → Mail → Provider → Webhook URLs** (Postmark only — self-hosted SMTP doesn't use webhooks).

**Self-hosted SMTP (built-in listener)**:

1. TinyCld's port 25 listener accepts the connection from the sending MTA.
2. RCPT TO is resolved against the same mailbox + alias tables. Unknown recipients are rejected with `550 5.1.1 No such mailbox` so the sending MTA bounces.
3. The DATA payload is parsed and dispatched to every matching mailbox via the same code path Postmark webhooks use.

**Self-hosted SMTP (IMAP fetcher)**:

1. The fetcher logs in to your configured IMAP server every poll interval.
2. Unseen messages are pulled and parsed.
3. Each recipient on the message is resolved against the mailbox + alias tables, **scoped to this org** (so a fetcher polling an IMAP account that receives mail for many domains only routes mail for domains this org owns).
4. Successfully ingested messages are marked `\Seen` so the next poll skips them.

## Removing a domain

Delete the domain row from the settings page. This removes it from TinyCld but doesn't touch your DNS records or the provider side — you'll want to remove those manually if the domain isn't going to be used elsewhere.

Mailboxes and aliases on the deleted domain are also removed (cascade), so be sure no one is depending on the addresses before deleting.

## See also

- [Provider setup](help://mail:provider-setup)
- [Mailboxes and aliases](help://mail:mailboxes)
- [Delivery tracking](help://mail:delivery-tracking)
