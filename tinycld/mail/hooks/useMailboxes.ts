import { eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
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

// A membership whose mailbox is gone, or not loaded yet, shows nothing.
function loadedMailboxes(rows: { mailbox?: MailMailboxes }[]) {
    return rows.flatMap(r => (r.mailbox ? [r.mailbox] : []))
}

export function useMailboxes() {
    const [membersCollection, mailboxesCollection] = useStore(
        'mail_mailbox_members',
        'mail_mailboxes'
    )

    // One query: membership rows resolve to their mailbox in the same
    // expression, so an optimistically-created mailbox appears immediately
    // instead of waiting for a second collection's realtime round-trip.
    //
    // A left join, not an inner join: it keeps the membership side active, so
    // the person's rows load and subscribe by `user` alone and the mailbox
    // loads by id. An inner join picks its active side by store size, and
    // when the mailboxes side won, memberships were subscribed only for the
    // mailboxes already known: access to a new inbox (an owner adds the person,
    // or a group grant) never arrived until a reload.
    const { data: rows, isLoading } = useMyLiveQuery(
        (query, { userId }) =>
            query
                .from({ member: membersCollection })
                .join({ mailbox: mailboxesCollection }, ({ member, mailbox }) =>
                    eq(member.mailbox, mailbox.id)
                )
                .where(({ member }) => eq(member.user, userId)),
        { queryKey: ['mail_mailboxes'] }
    )

    return useMemo(
        () => ({ ...splitMailboxes(loadedMailboxes(rows ?? [])), isLoading }),
        [rows, isLoading]
    )
}
