import { filterOwnAddresses, pickReplyFrom } from './defaultFromIdentity'
import type { SendableIdentity } from './groupSendableIdentities'
import type { MailMessages } from '../types'
import type { ReplyContext } from './useComposeState'

export type ReplyKind = 'reply' | 'replyAll' | 'forward'

export type ReplySource = Pick<
    MailMessages,
    | 'id'
    | 'subject'
    | 'sender_name'
    | 'sender_email'
    | 'recipients_to'
    | 'recipients_cc'
    | 'delivered_to'
>

interface BuildReplyContextParams {
    kind: ReplyKind
    message: ReplySource
    threadId: string
    threadMailboxId: string
    identities: SendableIdentity[]
}

function recipientsFor(kind: ReplyKind, message: ReplySource) {
    const sender = { name: message.sender_name, email: message.sender_email }
    if (kind === 'forward') return []
    if (kind === 'reply') return [sender]
    return [sender, ...message.recipients_to, ...message.recipients_cc]
}

/**
 * Pure: everything compose needs to start a reply, reply-all or forward of
 * one message. Shared by the thread screen's per-message actions and the
 * inline reply bar so both pick the same From.
 */
export function buildReplyContext({
    kind,
    message,
    threadId,
    threadMailboxId,
    identities,
}: BuildReplyContextParams): ReplyContext {
    const headerAddresses = [...message.recipients_to, ...message.recipients_cc].map(r => r.email)
    const from = pickReplyFrom({
        identities,
        deliveredToAddressId: message.delivered_to || null,
        headerAddresses,
        threadMailboxId,
    })
    // Reply-all leaves out the sending mailbox and my personal inbox: a reply
    // from Support must not copy me in. Other shared mailboxes stay — their
    // other members may still need the reply.
    const own = identities.filter(i => i.mailboxId === from.mailboxId || i.isPersonal)
    const raw = recipientsFor(kind, message)
    const to = kind === 'replyAll' ? filterOwnAddresses({ identities: own, recipients: raw }) : raw
    return {
        messageId: message.id,
        threadId,
        subject: kind === 'forward' ? `Fwd: ${message.subject}` : message.subject,
        to,
        mailboxId: threadMailboxId,
        from,
    }
}
