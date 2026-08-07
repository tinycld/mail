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
converted to plain text — add `--html` for the original markup.

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
handy at the end of a pipeline. Mail is sent from your first mailbox unless
you pick another with `--mailbox` (by address or id; `tinycld mail mailboxes`
lists yours).

## Keeping an eye on things

```
tinycld mail status     # unread and folder counts per mailbox
```

Every command accepts `--json` for stable, machine-readable output, so a cron
job can watch `mail status` or archive search results without screen-scraping
tables. Replying, drafts, and moving messages between folders are done in the
app for now.
