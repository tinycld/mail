---
title: Mail rules
summary: Filter, file, and reply to mail automatically
tags: [rules, automation, filters, workflow]
order: 100
---

Mail rules are filters: they watch for arriving mail and act on it without you
opening the message. See [Automation rules](help://core:rules) for how the
builder works — this topic covers what mail itself contributes.

## When a message arrives

Mail provides one trigger, **A message arrives**. It fires for genuinely
received mail only: drafts you're composing, messages you send, and bounce
notifications never start a rule.

You can filter on the message's subject, sender address, sender name, whether
it has attachments, and which alias it came in on. A rule with no conditions
runs on every arriving message.

## What a rule can do

| Action | What it does |
|---|---|
| **Move to folder** | Files the thread in Archive, Trash, Spam, or Inbox |
| **Mark as read** | Clears the unread state |
| **Forward the message** | Sends a copy to another address |
| **Star the message** | Stars the thread |
| **Send a message** | Sends new mail, with a subject and body you write |

Moving a message does not change whether it's read, marking it read does not
move it, and starring does neither — the actions are independent, so you can
combine them.

## When a message bounces

The trigger **A message bounces** fires when something you sent doesn't reach
its recipient — a hard bounce, or the recipient marking it as spam. It does not
fire on ordinary successful sends.

You can filter on the subject, the reason the delivery failed, and which
address it was sent from. Pair it with a notification if you send anything that
matters going unnoticed.

## Recipes

**File newsletters automatically.** When a message arrives, if the sender
contains `newsletter@`, move it to Archive and mark it as read. The mail never
touches your inbox, but it's still searchable.

**Forward invoices to accounting.** When a message arrives, if the subject
contains `invoice`, forward it to your accounting address.

**Auto-reply while away.** When a message arrives, send a message to
`{{sender_email}}` with the subject `Re: {{subject}}`. The `{{ }}` button in
the builder inserts these placeholders; they're replaced with the real values
when the rule runs.

**Add senders to your contacts.** When a message arrives, add a contact using
`{{sender_name}}` and `{{sender_email}}`. This needs the contacts package
installed, and it does not check for duplicates — a sender who mails you twice
can be added twice.

## Who a rule acts for

This differs between personal and organization rules, and the difference
matters most on a shared mailbox:

- **Personal rules** act only for you. Moving a message to Archive archives it
  in your view; everyone else in a shared mailbox still sees it in their inbox.
- **Organization rules** act for **every member of the mailbox**. An org rule
  that archives spam archives it for the whole team.

Org rules are admin-authored, so this is deliberate — a shared "archive spam"
rule that only tidied the inbox of whoever wrote it would be surprising. If you
want a rule that affects only you, make it a personal rule.

## Auto-replies won't loop

A rule that sends mail in response to arriving mail can, in principle, answer
another auto-responder forever. Two things stop that.

Mail caps how much a mailbox sends within an hour. Only outgoing mail counts —
messages you receive never use up the allowance. The cap is shared by all of
that mailbox's rules, so adding more reply rules doesn't raise it, and an
exchange between two auto-responders stops on its own. If you notice a reply
rule going quiet, check its run history: hitting the cap is recorded there.

A rule also can't send to the mailbox it's sending from. Both the mailbox's own
address and any of its aliases are refused, so a forward rule pointed at itself
can't feed itself. Forwarding or replying to any other address, including
`{{sender_email}}`, works normally.

## What rules can't do yet

- **Timing.** Rules react to mail as it arrives. There's no way to say "if this
  is still unread in three days" or "every morning at 9" against a specific
  message — scheduled rules exist, but they don't carry a message with them.
- **Starring as a trigger.** Starring is per-person state, so a rule started by
  a star can't read the message's subject or sender. There's no "when I star a
  message" trigger for that reason.
- **Digests.** "Send me one summary of today's mail" needs batching, which
  rules don't do — each message is handled on its own.
