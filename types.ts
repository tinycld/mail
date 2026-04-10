import type { Orgs, UserOrg } from '~/types/pbSchema'

export interface MailDomains {
    id: string
    org: string
    domain: string
    verified: boolean
    created: string
    updated: string
}

export interface MailMailboxes {
    id: string
    address: string
    domain: string
    display_name: string
    name: string
    type: 'personal' | 'shared'
    created: string
    updated: string
}

export interface MailMailboxMembers {
    id: string
    mailbox: string
    user_org: string
    role: 'owner' | 'member'
    created: string
    updated: string
}

export interface MailLabels {
    id: string
    org: string
    name: string
    color: string
    created: string
    updated: string
}

export interface MailThreads {
    id: string
    mailbox: string
    subject: string
    snippet: string
    message_count: number
    latest_date: string
    participants: { name: string; email: string }[]
    created: string
    updated: string
}

export interface MailMessages {
    id: string
    thread: string
    message_id: string
    in_reply_to: string
    sender_name: string
    sender_email: string
    recipients_to: { name: string; email: string }[]
    recipients_cc: { name: string; email: string }[]
    date: string
    subject: string
    snippet: string
    has_attachments: boolean
    total_size?: number
    body_html: string
    attachments: string[]
    delivery_status: 'sending' | 'sent' | 'delivered' | 'bounced' | 'spam_complaint' | 'draft'
    bounce_reason: string
    imap_uid: number
    raw_headers: string
    created: string
    updated: string
}

export interface MailThreadState {
    id: string
    thread: string
    user_org: string
    folder: 'inbox' | 'sent' | 'drafts' | 'trash' | 'spam' | 'archive'
    is_read: boolean
    is_starred: boolean
    labels: string[]
    created: string
    updated: string
}

export interface MailImapMailboxState {
    id: string
    mailbox: string
    uid_validity: number
    uid_next: number
    created: string
    updated: string
}

export type MailSchema = {
    mail_domains: {
        type: MailDomains
        relations: {
            org: Orgs
        }
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
            user_org: UserOrg
        }
    }
    mail_labels: {
        type: MailLabels
        relations: {
            org: Orgs
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
        }
    }
    mail_thread_state: {
        type: MailThreadState
        relations: {
            thread: MailThreads
            user_org: UserOrg
            labels: MailLabels[]
        }
    }
    mail_imap_mailbox_state: {
        type: MailImapMailboxState
        relations: {
            mailbox: MailMailboxes
        }
    }
}
