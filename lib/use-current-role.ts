import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { useAuth } from '~/lib/auth'
import { useStore } from '~/lib/pocketbase'
import { useOrgInfo } from '~/lib/use-org-info'

export function useCurrentRole() {
    const { user } = useAuth()
    const { orgId } = useOrgInfo()
    const [userOrgCollection] = useStore('user_org')

    const { data: userOrgs } = useLiveQuery(
        query =>
            query
                .from({ user_org: userOrgCollection })
                .where(({ user_org }) => and(eq(user_org.user, user.id), eq(user_org.org, orgId))),
        [user.id, orgId]
    )

    const userOrg = userOrgs?.[0]
    return {
        role: userOrg?.role ?? null,
        isAdmin: userOrg?.role === 'admin',
        userOrgId: userOrg?.id ?? '',
    }
}
