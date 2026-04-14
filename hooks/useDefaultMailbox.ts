import { eq } from '@tanstack/db'
import { useOrgLiveQuery, useStore } from '~/lib/pocketbase'

export function useDefaultMailbox() {
    const [membersCollection] = useStore('mail_mailbox_members')

    const { data: members } = useOrgLiveQuery((query, { userOrgId }) =>
        query
            .from({ mail_mailbox_members: membersCollection })
            .where(({ mail_mailbox_members }) => eq(mail_mailbox_members.user_org, userOrgId))
    )

    const firstMember = members?.[0]
    return firstMember?.mailbox ?? null
}
