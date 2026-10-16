---
title: Mailboxes, members, and addresses
summary: Personal vs shared mailboxes, managing access, and multiple addresses
tags: [mailbox, shared, address, alias, members, owner]
order: 20
---

## Personal mailboxes

When your account is created on a server that has Mail installed, a personal mailbox is created automatically. The address is derived from your **username**; you can rename it from **Settings → Mailboxes**. You're the **owner** of your personal mailbox and you're the only member — nobody else on the server can read it.

If your account is deleted, your personal mailbox and all its messages are deleted. This also applies if you shared the mailbox with other people: they lose access to it. Its messages do not go to the person who takes over your content.

## Shared mailboxes

A shared mailbox is for an address that multiple people work out of — `support@`, `sales@`, `team@`. To create one:

1. Open **Settings → Mailboxes**.
2. Click **+ New mailbox**.
3. Pick **Shared** as the type and fill in the local part of the address, display name, and domain.
4. Click **Save**.

You're automatically the owner. To add other members, open the mailbox row and click **Add member**.

## Roles

Every mailbox member has one of two roles:

- **Owner** — can add and remove members, rename the mailbox, manage addresses, and delete it.
- **Member** — can read and send from the mailbox, but can't change membership or settings.

You can't have a mailbox with zero owners — removing the last owner is blocked. If you need to transfer a mailbox, promote someone to owner first, then remove yourself.

If you are the only owner of a shared mailbox that other people use, you cannot delete your account until the mailbox has another owner. Make one of its members an owner or delete the mailbox. You can also choose someone to take over your content when you delete your account; that person then becomes the owner.

## Addresses

A mailbox can have more than one address, on any of your domains. Mail sent to any of them arrives in the same mailbox.

To add an address, open the mailbox in **Settings → Mail → Mailboxes**, open **Addresses**, and click **Add address**. Type the part before the `@` and choose the domain.

The list marks two kinds of address that you cannot remove there:

- **Main address** — the mailbox's own address. To change it, edit the mailbox.
- **From the domain rule** — given to each person by a domain set to **Everyone**. To remove these, change the domain's setting in [Custom domains](help://mail:custom-domains).

You can remove only addresses that were added by hand.

On a domain set to **Everyone**, a person's email name is reserved for that person, so a shared mailbox cannot use it there.

When you compose, the **From** picker lets you choose which address to send as. Replies use whichever address the incoming message was sent *to*, so threading stays clean.

## What every member sees

When you join a shared mailbox, you immediately see:

- Existing threads in their folders (the actual message data — Inbox, Sent, etc.).
- New incoming messages in real time.

But each member has their own:

- Read / unread state.
- Starred flag.
- Folder placement (you can archive a thread without affecting other members).
- Label assignments.

So if Alice and Bob both have access to `support@`, Alice can star a thread without changing what Bob sees, and Bob can move it to Archive while Alice keeps it in Inbox.

## See also

- [Provider setup](help://mail:provider-setup)
- [Custom domains](help://mail:custom-domains)
- [Folders](help://mail:folders)
