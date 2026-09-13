import { eq } from '@tanstack/db'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'

export function useDefaultMailbox() {
    const [membersCollection] = useStore('mail_mailbox_members')

    const { data: members } = useMyLiveQuery((query, { userId }) =>
        query
            .from({ mail_mailbox_members: membersCollection })
            .where(({ mail_mailbox_members }) => eq(mail_mailbox_members.user, userId))
    )

    const firstMember = members?.[0]
    return firstMember?.mailbox ?? null
}
