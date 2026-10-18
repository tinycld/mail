---
title: Your email addresses
summary: Where your addresses come from, your email name, your Default From, and the address a reply uses
tags: [addresses, email name, default from, reply]
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
- **Shared inboxes** — the shared inboxes that the person is a member of. This part shows only when the person is a member of one.

A guest does not get email addresses. When a person leaves, TinyCld removes their addresses.

To manage inboxes that a team uses together, see [Shared inboxes](help://mail:mailboxes).
