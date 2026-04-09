import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useStore } from '~/lib/pocketbase'
import { useCurrentRole } from '~/lib/use-current-role'

export function useDefaultMailbox() {
    const { userOrgId } = useCurrentRole()
    const [membersCollection] = useStore('mail_mailbox_members')

    const { data: members } = useLiveQuery(
        query =>
            query
                .from({ mail_mailbox_members: membersCollection })
                .where(({ mail_mailbox_members }) => eq(mail_mailbox_members.user_org, userOrgId)),
        [userOrgId]
    )

    const firstMember = members?.[0]
    return firstMember?.mailbox ?? null
}
