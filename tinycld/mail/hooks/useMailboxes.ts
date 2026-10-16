import { eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { materialize } from 'pbtsdb'
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

    // The person's own membership rows drive the query: they load and
    // subscribe by `user` alone, so access to a new inbox (an owner adds the
    // person, or a group grant) arrives through this query without a reload.
    // Each row reads its mailbox from its own collection by id. An inner join
    // did not do this: it picks its active side by store size, and when the
    // mailboxes side won, memberships were subscribed only for the mailboxes
    // already known.
    const { data: rows, isLoading } = useMyLiveQuery(
        (query, { userId }) =>
            query
                .from({ member: membersCollection })
                .where(({ member }) => eq(member.user, userId))
                .select(({ member }) => ({
                    id: member.id,
                    mailbox: materialize(
                        query
                            .from({ mailbox: mailboxesCollection })
                            .where(({ mailbox }) => eq(mailbox.id, member.mailbox))
                            .findOne()
                    ),
                })),
        { queryKey: ['mail_mailboxes'] }
    )

    return useMemo(() => {
        type Row = NonNullable<typeof rows>[number]
        // A membership whose mailbox is gone, or not loaded yet, shows nothing.
        const loaded = (row: Row) => (row.mailbox ? [row.mailbox] : [])
        return { ...splitMailboxes((rows ?? []).flatMap(loaded)), isLoading }
    }, [rows, isLoading])
}
