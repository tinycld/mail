import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import type { GroupMemberRemovalNoticeProps } from '@tinycld/core/lib/groups/removal-notice'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMemo } from 'react'
import { Text } from 'react-native'
import { addressesLost, type GroupAddressRow, removalNoticeText } from './group-removal-notice'

function useRemovalNotice({ groupId, userId }: GroupMemberRemovalNoticeProps) {
    const [
        addressesCollection,
        domainsCollection,
        identitiesCollection,
        usersCollection,
        membersCollection,
    ] = useStore('mail_addresses', 'mail_domains', 'mail_identities', 'users', 'group_members')
    // One row per (address, owner's group membership): the memberships tell
    // whether another of the domain's groups keeps the address.
    const { data: rows } = useLiveQuery({
        queryKey: ['mail_group_removal_addresses', groupId, userId],
        query: query =>
            query
                .from({ address: addressesCollection })
                .innerJoin({ domain: domainsCollection }, ({ address, domain }) =>
                    eq(address.domain, domain.id)
                )
                .innerJoin({ identity: identitiesCollection }, ({ address, identity }) =>
                    eq(address.mailbox, identity.inbox)
                )
                .innerJoin({ user: usersCollection }, ({ identity, user }) =>
                    eq(identity.user, user.id)
                )
                .join({ member: membersCollection }, ({ identity, member }) =>
                    eq(identity.user, member.user)
                )
                .where(({ address, identity }) => {
                    const fromGroup = and(
                        eq(address.via_group, groupId),
                        eq(address.source, 'rule')
                    )
                    if (userId === null) return fromGroup
                    return and(fromGroup, eq(identity.user, userId))
                })
                .select(({ address, domain, identity, user, member }) => ({
                    localPart: address.local_part,
                    domainName: domain.domain,
                    coverageGroups: domain.coverage_groups,
                    userId: identity.user,
                    name: user.name,
                    email: user.email,
                    memberGroup: member?.group,
                })),
    })
    return useMemo(() => {
        const addressRows: GroupAddressRow[] = []
        const memberships = new Map<string, string[]>()
        for (const row of rows ?? []) {
            addressRows.push({
                email: `${row.localPart}@${row.domainName}`,
                userId: row.userId,
                coverageGroups: row.coverageGroups,
            })
            const groups = memberships.get(row.userId) ?? []
            if (!row.memberGroup || groups.includes(row.memberGroup)) continue
            memberships.set(row.userId, [...groups, row.memberGroup])
        }
        const person = rows?.[0]
        const personName = userId === null || !person ? null : person.name || person.email
        return removalNoticeText({
            personName,
            addresses: addressesLost(addressRows, memberships, groupId),
        })
    }, [rows, userId, groupId])
}

/** Contributed to core's group-member-removal slot: the mail a person or the members lose. */
export default function GroupRemovalNotice(props: GroupMemberRemovalNoticeProps) {
    const text = useRemovalNotice(props)
    if (!text) return null
    return <Text className="text-muted-foreground text-sm">{text}</Text>
}
