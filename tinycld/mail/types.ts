import type { Recipient, VerificationDetails } from '@tinycld/app-generated/mail-api'
import type {
    MailDomains as GenMailDomains,
    MailFolderCounts as GenMailFolderCounts,
    MailImapMailboxState as GenMailImapMailboxState,
    MailMailboxAliases as GenMailMailboxAliases,
    MailMailboxes as GenMailMailboxes,
    MailMailboxMembers as GenMailMailboxMembers,
    MailMessages as GenMailMessages,
    MailThreadState as GenMailThreadState,
    MailThreads as GenMailThreads,
    Users,
} from '@tinycld/core/types/pbSchema'

// The generated pbSchema interfaces are the source of truth for collection
// fields, but PocketBase JSON columns carry no shape information and emit
// `any`. Each interface here derives from its generated counterpart and
// overrides only the JSON fields with their real shapes — taken from the
// generated payload contract (@tinycld/app-generated/mail-api) where one
// exists. Collections without JSON fields are plain re-exports, so a
// migration-driven field change flows through on the next install instead of
// silently drifting from a hand-written copy.

export interface MailDomains extends Omit<GenMailDomains, 'verification_details'> {
    verification_details: VerificationDetails | null
}

export type MailMailboxes = GenMailMailboxes
export type MailMailboxMembers = GenMailMailboxMembers
export type MailMailboxAliases = GenMailMailboxAliases
export type MailThreadState = GenMailThreadState
export type MailImapMailboxState = GenMailImapMailboxState

export interface MailThreads extends Omit<GenMailThreads, 'participants'> {
    participants: Recipient[]
}

export interface MailMessages
    extends Omit<
        GenMailMessages,
        | 'recipients_to'
        | 'recipients_cc'
        | 'recipients_bcc'
        | 'cid_map'
        | 'attachment_thumbnail_map'
    > {
    recipients_to: Recipient[]
    recipients_cc: Recipient[]
    recipients_bcc: Recipient[]
    cid_map: Record<string, string> | null
    attachment_thumbnail_map: Record<string, string> | null
}

// View collection: the aggregate count columns come back untyped from the
// schema generator, but they are always numbers.
export interface MailFolderCounts
    extends Omit<GenMailFolderCounts, 'inbox' | 'drafts' | 'sent' | 'starred' | 'trash' | 'spam'> {
    inbox: number
    drafts: number
    sent: number
    starred: number
    trash: number
    spam: number
}

export type MailSchema = {
    mail_domains: {
        type: MailDomains
        relations: Record<string, never>
    }
    mail_mailboxes: {
        type: MailMailboxes
        relations: {
            domain: MailDomains
        }
    }
    mail_mailbox_members: {
        type: MailMailboxMembers
        relations: {
            mailbox: MailMailboxes
            user: Users
        }
    }
    mail_mailbox_aliases: {
        type: MailMailboxAliases
        relations: {
            mailbox: MailMailboxes
        }
    }
    mail_threads: {
        type: MailThreads
        relations: {
            mailbox: MailMailboxes
        }
    }
    mail_messages: {
        type: MailMessages
        relations: {
            thread: MailThreads
            alias: MailMailboxAliases
        }
    }
    mail_thread_state: {
        type: MailThreadState
        relations: {
            thread: MailThreads
            user: Users
        }
    }
    mail_imap_mailbox_state: {
        type: MailImapMailboxState
        relations: {
            mailbox: MailMailboxes
        }
    }
    mail_folder_counts: {
        type: MailFolderCounts
        relations: {
            user: Users
            mailbox: MailMailboxes
        }
    }
}
