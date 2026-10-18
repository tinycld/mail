---
title: Composing and sending email
summary: The compose window, attachments, and choosing which address to send from
tags: [compose, send, draft, attachment, from]
order: 30
---

## Opening compose

Three ways:

- Click **Compose** at the top of the sidebar.
- Press **c** anywhere in Mail.
- Click **Reply** or **Forward** on an open thread (this is technically [inline reply](#inline-replies), see below).

The compose window opens in the bottom-right corner. You can move it, minimize it, or pop it out into a full-screen overlay. Multiple compose windows can be open at once — useful for drafting a reply while you check another thread.

## Filling the form

- **From** — the address you send as. A new message starts from your Default From. Click **Change** to choose any address of your own inbox or of a shared inbox you belong to. A reply or forward starts from the address the original message was sent to; for older mail, from one of your addresses in its To or Cc. See [Your email addresses](help://mail:addresses). An open draft keeps its inbox: you can change its From only to another address of the same inbox.
- **To** — primary recipients. Type and tab/comma to add multiple. Recipients auto-suggest from [Contacts](help://contacts:getting-started) if you have it installed.
- **Cc / Bcc** — secondary recipients. Hidden by default; click **Cc Bcc** in the header to reveal. Bcc recipients are saved with the draft and kept on your Sent copy, so you can see later who was blind-copied (the recipients themselves never see the Bcc list).
- **Subject** — required.
- **Body** — rich-text editor with bold, italic, lists, links, and inline images.

## Attachments

Drag files onto the compose window or click the paperclip icon (on a narrow compose window it may sit under **More** in the toolbar). Attachments:

- Are uploaded as part of the message — no separate upload step.
- Are limited to 20 files and 10 MB in total for each message. This includes the files that you saved with a draft.
- Show as a ribbon at the bottom of the compose window; click the **×** on any attachment to remove it.

## Inline images

Drag an image into the body to embed it inline (vs as a separate attachment). The image gets a content-ID and is referenced via `cid:` in the HTML body. Recipients see it inline regardless of whether their client renders external images.

## Drafts

A draft is saved when you close the compose window. Drafts are in the **Drafts** folder of the inbox that you send from. To continue a draft, open it from Drafts. A reopened draft shows the files that you attached to it. When you send the draft, these files go with the message. If you remove a file, the file is removed from the draft when you close the compose window. If a saved file cannot be read, the message is not sent and the draft stays. If another person deletes the draft while you have it open, its saved files are deleted too. To send the message, remove those files from the message, attach them again, and then click **Send**. To close the compose window without saving your changes, click **Discard**. A draft that you saved before stays in Drafts. To delete a draft, select it in Drafts and click **Delete**. The draft moves to Trash.

## Sending

Click **Send**. The message goes through this server's mail provider (Postmark or self-hosted SMTP — see [Provider setup](help://mail:provider-setup)), and:

- Immediately appears in **Sent** for the sending inbox.
- Gets a `delivery_status` of `sending`, updated to `sent` once the provider accepts it, then to `delivered`, `bounced`, or `spam_complaint` based on provider callbacks. See [Delivery tracking](help://mail:delivery-tracking).

If the deployment has no mail provider configured, sending fails with a clear error. Configuring one is an administrator task; on a deployment run by a service provider it is already done for you.

### Storage limit

Stored messages — bodies and attachments — count toward this server's storage limit. If saving a draft or sending would push the total over the limit, the server refuses it with a `storage limit exceeded` error that shows how much is used, the limit, and how much the message needed. Free space by deleting large threads from Trash, or ask an administrator to raise the limit.

## Inline replies

Replies don't open a new compose window — they expand inline at the bottom of the thread. The same form is there, pre-filled with the recipient, subject (with `Re:`), and quoted history. Send and the thread refreshes with the reply at the end.

## Keyboard shortcuts

In the compose window:

| Shortcut | Action |
|---|---|
| ⌘⏎ | Send |
| ⌘B / ⌘I / ⌘U | Bold / italic / underline |
| ⌘K | Add a link |

## See also

- [Reading threads](help://mail:reading-threads)
- [Delivery tracking](help://mail:delivery-tracking)
- [Your email addresses](help://mail:addresses)
- [Shared inboxes](help://mail:mailboxes)
