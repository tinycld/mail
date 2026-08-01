import { eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { useMemo } from 'react'
import type { MailMailboxes } from '../types'
import { splitMailboxes } from './splitMailboxes'

export type { MailboxesResult } from './splitMailboxes'
export { splitMailboxes }

export function getMailboxLabel(
    mailbox: Pick<MailMailboxes, 'display_name' | 'address'>,
    isPersonal: boolean
): string {
    if (isPersonal) return 'Personal'
    return mailbox.display_name || mailbox.address
}

export function useMailboxes() {
    const [membersCollection, mailboxesCollection] = useStore(
        'mail_mailbox_members',
        'mail_mailboxes'
    )

    // One query: membership rows resolve to their mailbox in the same
    // expression, so an optimistically-created mailbox appears immediately
    // instead of waiting for a second collection's realtime round-trip.
    const { data: rows } = useOrgLiveQuery((query, { userId }) =>
        query
            .from({ member: membersCollection })
            .innerJoin({ mailbox: mailboxesCollection }, ({ member, mailbox }) =>
                eq(member.mailbox, mailbox.id)
            )
            .where(({ member }) => eq(member.user, userId))
    )

    return useMemo(() => splitMailboxes((rows ?? []).map(r => r.mailbox)), [rows])
}
