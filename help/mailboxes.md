---
title: Shared inboxes
summary: Inboxes that a team uses together, their members, roles, and addresses
tags: [shared inbox, inbox, shared, address, members, owner]
order: 20
---

A shared inbox is for an address that a team uses together, for example `support@`, `sales@`, or `team@`. All members can read the mail and send from the inbox.

Each person also has a personal inbox. TinyCld makes it when the account is made. You manage a personal inbox in **Settings → Members**, not here. Click the person and use the **Email** section. See [Your email addresses](help://mail:addresses).

## What an admin sees

An admin or an owner of the workspace sees every shared inbox in **Settings → Mail → Shared inboxes**, with its members. An admin cannot read the mail of an inbox, unless the admin is a member of it. Only an owner of the inbox can change its members and settings.

## Make a shared inbox

1. Open **Settings → Mail → Shared inboxes**.
2. Click **New shared inbox**.
3. Type the part of the address before the `@`, choose the domain, and type a display name.
4. Click **Create inbox**.

You become the owner. To add other members, click the inbox row, open **Members**, and click **Add member**.

## Roles

Each member of a shared inbox has one of two roles:

- **Owner** — can add and remove members, edit the inbox, manage its addresses, and delete it.
- **Member** — can read and send from the inbox, but cannot change the members or the settings.

A shared inbox must always have an owner. You cannot remove the last owner. To give the inbox to a different person, first make that person an owner, then remove yourself.

If you are the only owner of a shared inbox that other people use, you cannot delete your account until the inbox has another owner. Make one of its members an owner, or delete the inbox. You can also choose a person to take your content when you delete your account. That person then becomes the owner.

If a person's account is deleted, their personal inbox and all of its messages are deleted. The messages do not go to the person who takes the content.

## Give a group access

You can give all of the people in a group access to a shared inbox. You must be an owner of the inbox.

1. Open **Settings → Mail → Shared inboxes** and click the inbox row.
2. Open **Members**.
3. Under **Groups**, click **Add group**.
4. Find the group and click **Add**.

Each person in the group can then read the mail and send from the inbox. **Members** shows each of these people with **Through group _name_**.

Group access is always **Member**. A group cannot be an owner. To make a person in the group an owner, add that person directly with **Add member**, then make the person an owner.

When a person leaves the group, the person loses access to the inbox. When you remove the group from the inbox, all of its people lose access. A person who is also a direct member, or a member through a different group, keeps access.

To change the people in a group, open **Settings → Groups**.

## Addresses

A shared inbox can have more than one address, on any of your domains. Mail sent to each address arrives in the same inbox.

To add an address, click the inbox row in **Settings → Mail → Shared inboxes**, open **Addresses**, and click **Add address**. Type the part before the `@` and choose the domain.

The list marks the addresses that you cannot remove there:

- **Main address** — the inbox's own address. To change it, edit the inbox.
- **From the domain rule** and **From group _name_** — addresses that a domain setting gives. To remove them, change the domain setting in [Custom domains](help://mail:custom-domains), or the group in **Settings → Groups**.

You can remove only the addresses that a person added by hand.

On a domain set to **Everyone** or **Members of these groups**, each person's email name is kept for that person. A shared inbox cannot use the email name as an address on that domain.

When you compose, you can choose the address to send from in the **From** list. A reply uses the address that received the message. See [Your email addresses](help://mail:addresses).

## What each member sees

When you join a shared inbox, you see immediately:

- The threads in their folders (Inbox, Sent, and the other folders).
- New messages when they arrive.

Each member has their own:

- Read and unread state.
- Starred flag.
- Folder placement. You can archive a thread, and the thread stays in the Inbox for other members.
- Labels.

For example, Alice and Bob are members of `support@`. Alice can star a thread, and Bob does not see the star. Bob can move the thread to Archive, and the thread stays in Alice's Inbox.

## See also

- [Your email addresses](help://mail:addresses)
- [Provider setup](help://mail:provider-setup)
- [Custom domains](help://mail:custom-domains)
- [Folders](help://mail:folders)
