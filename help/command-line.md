---
title: Mail from the command line
summary: Search, read, and send mail from a terminal with the tinycld CLI.
tags: [cli, terminal, automation, search, send]
order: 150
---

The `tinycld` command line tool includes a `mail` command group when the Mail
package is installed. To download the tool and log in, see
[Command line tool](help://core:command-line). Everything below assumes you
are logged in.

## Finding and reading mail

```
tinycld mail list                       # your inbox, newest first
tinycld mail list --folder sent         # any folder: sent, drafts, trash, spam, archive, starred
tinycld mail search "invoice" --from billing --has-attachment
tinycld mail read <thread or message id>
```

`list` and `search` print thread ids in the last column; pass one to `read`
to print every message in the thread, oldest first. Message bodies are
converted to plain text — add `--html` for the original markup, or `--raw` for
the original headers (stored only for mail your client appended over IMAP).

The search flags mirror the app's advanced search: `--from`, `--to`,
`--subject`, `--has-words`, `--date-after`, `--date-before`, `--folder`, and
`--has-attachment`. See [Searching your mail](help://mail:search) for what
each field matches.

## Attachments

```
tinycld mail attachments <message id>          # numbered list
tinycld mail download <message id> --out ./in  # save all of them
tinycld mail download <message id> --attachment 2
```

## Sending

```
tinycld mail send --to "Ada <ada@example.com>" \
    --subject "Quarterly report" \
    --body "Attached as promised." \
    --attach report.pdf
```

`--to`, `--cc`, and `--bcc` repeat for multiple recipients and accept either
a bare address or `Name <address>`. For a longer message, `--body-file
notes.txt` reads from a file and `--body-file -` reads from standard input —
handy at the end of a pipeline.

Mail goes out from your first mailbox unless you choose otherwise. `--from`
takes a full address and can select one of your aliases; `--mailbox` takes a
mailbox and always uses its own address. `tinycld mail mailboxes` lists what
you can send as.

## Replying

```
tinycld mail reply <thread-id> --body "On it."
tinycld mail reply <message-id> --all --body "Thanks all."
```

A thread id answers its most recent message. The subject gets a `Re:` prefix
(never doubled), and the reply is threaded with the original. Plain `reply`
writes to the sender; `--all` adds everyone in To and Cc, minus your own
address.

## Drafts

```
tinycld mail draft --to ada@example.com --subject "Later" --body "wip"
tinycld mail draft --message-id <draft-id> --body "revised"
tinycld mail draft send <draft-id>
```

`draft` saves a message without sending it, and `--message-id` revises one you
already saved. `draft send` sends the saved contents — attachments included —
and removes the draft. See [Composing](help://mail:composing).

## Organizing

```
tinycld mail archive <thread-id>...        # also: trash, spam
tinycld mail move inbox <thread-id>...     # inbox, sent, drafts, trash, spam, archive
tinycld mail mark read <thread-id>...      # or: unread
tinycld mail star <thread-id>...           # or: unstar
```

Each of these takes any number of threads. They change only your view: a
co-member of a shared mailbox still sees the thread where they left it.

```
tinycld mail labels                        # labels you can use
tinycld mail label add <thread-id> Work
tinycld mail label remove <thread-id> Work
```

Labels are matched by name (case-insensitive) or id, and are created in the
app. See [Labels](help://mail:labels) and [Folders](help://mail:folders).

## Keeping an eye on things

```
tinycld mail status     # unread and folder counts per mailbox
```

Every command accepts `--json` for stable, machine-readable output, so a cron
job can watch `mail status` or archive search results without screen-scraping
tables.

Note that `read` marks a thread as read, exactly as opening it in the app
does. Pass `--no-mark` when a script should look without disturbing your
unread counts.
