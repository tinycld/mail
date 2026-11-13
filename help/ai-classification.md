---
title: AI classification
summary: Let AI put one label on each new message, and optionally archive low-value mail.
tags: [ai, labels, classification, archive, rules]
order: 40
---

AI classification reads each new message and gives it one label from a list of plain-English rules. Your admin must first turn it on in **Settings → AI classification**.

## Turn it on for yourself

1. Open **Settings → Account → AI classification**.
2. Set **Mode**:
   - **Preview only**: messages are classified, but nothing changes. Use this first, and check **Recent classifications**.
   - **Labels**: each new message gets its label.
   - **Labels and archive**: also archives a new conversation when its label is archive eligible and the AI is very sure.
3. Choose **Organization default** labels, or **My own labels** to write your own.

## When mail is archived

All of these must be true: your mode is **Labels and archive**, the label is **Archive eligible**, the confidence is at or above the archive threshold (your admin sets it; the minimum is 0.5 and the default is 0.9), the message starts a new conversation, and the conversation is still in your Inbox. A reply in an existing conversation is never archived. Archived mail does not send a notification.

## Write good rules

1. Describe what you can see in the message: "asks for pricing or a demo" is better than "good lead".
2. Say what the label excludes when it can overlap another label.
3. Keep labels clearly different from each other.
4. Describe urgency instead of using the word "urgent".
5. Keep a safe catch-all label, such as **review**, that is not archive eligible.
6. Make only clearly unwanted labels archive eligible.

## Use AI labels in rules

Create a rule with the trigger **AI classifies a message**, then add conditions on **AI label**, **Confidence**, or **Archived by AI**. Personal rules can also use **I have sent email to the sender** and, when the contacts app is installed, **Sender is in my contacts**. Personal rules can use these two conditions with the trigger **A message arrives** as well.

## What is sent

The label rules, the sender, recipients, subject, a short snippet and mailing-list headers go to OpenRouter. The message text is sent only when the first answer is not sure enough. Attachments are never sent.
