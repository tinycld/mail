import { eq } from '@tanstack/db'
import { useMemo } from 'react'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import type { MailMailboxes } from '../types'
import { splitMailboxes } from './splitMailboxes'

export { splitMailboxes }
export type { MailboxesResult } from './splitMailboxes'

export function useMailboxes() {
    const [membersCollection, mailboxesCollection] = useStore(
        'mail_mailbox_members',
        'mail_mailboxes'
    )

    const { data: members } = useOrgLiveQuery((query, { userOrgId }) =>
        query
            .from({ mail_mailbox_members: membersCollection })
            .where(({ mail_mailbox_members }) => eq(mail_mailbox_members.user_org, userOrgId))
    )

    const { data: mailboxes } = useOrgLiveQuery((query) =>
        query.from({ mail_mailboxes: mailboxesCollection })
    )

    return useMemo(() => {
        const ids = (members ?? []).map((m) => m.mailbox)
        return splitMailboxes(ids, (mailboxes ?? []) as MailMailboxes[])
    }, [members, mailboxes])
}
