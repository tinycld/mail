---
title: Your email addresses
summary: Where your addresses come from, your email name, your Default From, a folder for each address, the address a reply uses, and group addresses
tags: [addresses, email name, default from, address folders, reply, group address]
order: 15
---

To see your addresses, open **Settings → Account → Email**. Mail sent to each address in the list arrives in your inbox.

## Where your addresses come from

Your inbox has one **Main address**. TinyCld makes it on the main domain when your account is made. You can also get more addresses in three ways:

- **From the domain rule** — an admin set a domain to **Everyone**. Each person gets their email name on that domain.
- **From group _name_** — an admin set a domain to **Members of these groups**, and you are a member of the group. If you leave all of the groups, the address stops receiving mail.
- **Added by an admin** — an admin added the address by hand. This address has no label in the list.

For more about the domain settings, see [Custom domains](help://mail:custom-domains).

## Your email name

Your email name is the part before the `@` in the addresses that the domain rules give you. For example, if your email name is `nathan`, a domain `example.com` set to **Everyone** gives you `nathan@example.com`.

Only an admin can change an email name. When the email name changes, your main address and each address from a domain rule or a group change to the new name. For example, if `nathan` becomes `nate`, the main address `nathan@example.com` becomes `nate@example.com`.

The **Keep the old addresses as extra addresses** switch is on by default. When the switch is on, the old main address and the old rule addresses stay as extra addresses and continue to receive mail. When the switch is off, the old addresses stop receiving mail.

If your Default From is your main address, it changes to the new main address. If your Default From is an old rule address, it stays on that old address when the switch is on. When the switch is off, it changes to **The main domain's address**.

## Choose your Default From

Your Default From is the address that a new message starts from.

1. Open **Settings → Account → Email**.
2. Under **Send new mail from**, click an address.

TinyCld saves the change immediately. If you click **The main domain's address**, new mail starts from your address on the main domain. The main domain is the first verified domain in **Settings → Mail → Domains**.

To send one message from a different address, click **Change** next to **From** in the compose window. See [Composing](help://mail:composing).

## Show a folder for each address

When you have two or more addresses, you can show a folder for each address in the sidebar.

1. Open **Settings → Account → Email**.
2. Turn on **Show a folder for each address**.

The sidebar then shows one folder for each address, under **Inbox** in your personal inbox. The number on a folder is the number of unread conversations in it.

A folder shows the conversations in your **Inbox** that use its address: mail sent to that address, and mail that you sent from it. A folder is not a second inbox. Each conversation stays in your **Inbox**. If you archive or delete a conversation, it goes out of the folder too. A conversation that uses two of your addresses shows in the two folders.

To remove one address from the sidebar, set its switch in the **Show in sidebar** column to off. The folders of the other addresses stay. Mail to that address continues to arrive in your **Inbox**.

To remove all of the folders, turn off **Show a folder for each address**. Your mail app shows the same folders. See [Connecting an IMAP client](help://mail:imap).

## Which address a reply uses

A reply or a forward does not use your Default From. TinyCld chooses the From address in this sequence:

1. The address that received the message.
2. If TinyCld does not know that address (older mail), one of your addresses in the **To** or **Cc** of the message. The addresses of the inbox that holds the thread come first.
3. If no address matches, the main address of the inbox that holds the thread.

Thus the person who wrote to you sees the reply come from the address that they wrote to.

## For admins: manage a person's addresses

To manage the addresses of one person, open **Settings → Members** and click the person. The **Email** section of the drawer shows:

- **Addresses** — all of the person's addresses. To add an address, click **Add address**, type the part before the `@`, choose the domain, and click **Add address**. To remove an address that an admin added, click the **×** on its row. You cannot remove an address that comes from a domain rule or a group there. Change the domain setting or the group instead.
- **Email name** — to change it, click **Edit**, type the new name, set the **Keep the old addresses as extra addresses** switch, and click **Rename**.
- **Send new mail from** — the person's Default From.
- **Shared inboxes** — the shared inboxes that the person is a member of. If a group gives the access, the row shows the name of that group. This part shows only when the person is a member of one.

A guest does not get email addresses. When a person leaves, TinyCld removes their addresses.

To manage inboxes that a team uses together, see [Shared inboxes](help://mail:mailboxes).

## Give a group its own address

An admin can give a group its own address, for example `team@example.com`. Mail sent to this address goes to each member of the group.

1. Open **Settings → Groups** and click the group.
2. In the **Email** section, under **Group addresses**, click **Add address**.
3. Type the part before the `@` and choose the domain.
4. Click **Add address**.

To remove a group address, click the **×** on its row.

Each member gets a copy of the message in their own inbox. When you add a person to the group, the person gets the mail that arrives after that time. When you remove a person, the person stops getting the mail.

A reply to the message goes to the person who sent it. It does not go to the group.

Nobody can send mail from a group address. The group address is not in the **From** list.

When you delete the group, TinyCld deletes its addresses too.

To let a team read and answer the same mail together, use a shared inbox. See [Shared inboxes](help://mail:mailboxes).
