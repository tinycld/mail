import { eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'

export function useDefaultMailbox() {
    const [membersCollection] = useStore('mail_mailbox_members')

    const { data: members } = useOrgLiveQuery((query, { userId }) =>
        query
            .from({ mail_mailbox_members: membersCollection })
            .where(({ mail_mailbox_members }) => eq(mail_mailbox_members.user_org, userId))
    )

    const firstMember = members?.[0]
    return firstMember?.mailbox ?? null
}
