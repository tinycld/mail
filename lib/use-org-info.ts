import { useActiveParams } from 'one'
import { useLiveQuery } from '@tanstack/react-db'
import { eq } from '@tanstack/db'
import { useStore } from '~/lib/pocketbase'

export function useOrgInfo() {
    const { orgSlug = '' } = useActiveParams<{ orgSlug: string }>()
    const [orgsCollection] = useStore('orgs')

    const { data: orgs } = useLiveQuery(
        (query) =>
            query
                .from({ orgs: orgsCollection })
                .where(({ orgs }) => eq(orgs.slug, orgSlug)),
        [orgSlug],
    )

    const org = orgs?.[0] ?? null

    return { orgSlug, orgId: org?.id ?? '', org }
}
