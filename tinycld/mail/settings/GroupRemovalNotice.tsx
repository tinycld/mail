import { and, eq, not } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import type { GroupMemberRemovalNoticeProps } from '@tinycld/core/lib/groups/removal-notice'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMemo } from 'react'
import { Text } from 'react-native'
import {
    addressesLost,
    type GroupAddressRow,
    type InboxAccessRow,
    inboxesLost,
    removalNoticeText,
} from './group-removal-notice'

// The rows of the person (or of every member) on the inboxes the group grants.
function useInboxRows({ groupId, userId }: GroupMemberRemovalNoticeProps) {
    const [membersCollection, mailboxesCollection, domainsCollection, usersCollection] = useStore(
        'mail_mailbox_members',
        'mail_mailboxes',
        'mail_domains',
        'users'
    )
    const { data } = useLiveQuery({
        queryKey: ['mail_group_removal_inboxes', groupId, userId],
        query: query => {
            const grants = query
                .from({ grant: membersCollection })
                .where(({ grant }) => and(eq(grant.group, groupId), eq(grant.user, '')))
            return query
                .from({ member: membersCollection })
                .innerJoin({ grant: grants }, ({ member, grant }) =>
                    eq(member.mailbox, grant.mailbox)
                )
                .innerJoin({ mailbox: mailboxesCollection }, ({ member, mailbox }) =>
                    eq(member.mailbox, mailbox.id)
                )
                .innerJoin({ domain: domainsCollection }, ({ mailbox, domain }) =>
                    eq(mailbox.domain, domain.id)
                )
                .innerJoin({ user: usersCollection }, ({ member, user }) =>
                    eq(member.user, user.id)
                )
                .where(({ member }) =>
                    userId === null ? not(eq(member.user, '')) : eq(member.user, userId)
                )
                .select(({ member, mailbox, domain, user }) => ({
                    mailboxId: mailbox.id,
                    address: mailbox.address,
                    domainName: domain.domain,
                    userId: member.user,
                    viaGroup: member.group,
                    name: user.name,
                    email: user.email,
                }))
        },
    })
    return data
}

function useRemovalNotice(props: GroupMemberRemovalNoticeProps) {
    const { groupId, userId } = props
    const inboxRows = useInboxRows(props)
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
        const person = rows?.[0] ?? inboxRows?.[0]
        const accessRows: InboxAccessRow[] = (inboxRows ?? []).map(row => ({
            mailboxId: row.mailboxId,
            email: `${row.address}@${row.domainName}`,
            userId: row.userId,
            viaGroup: row.viaGroup,
        }))
        const personName = userId === null || !person ? null : person.name || person.email
        return removalNoticeText({
            personName,
            addresses: addressesLost(addressRows, memberships, groupId),
            inboxes: inboxesLost(accessRows, groupId),
        })
    }, [rows, inboxRows, userId, groupId])
}

/** Contributed to core's group-member-removal slot: the mail a person or the members lose. */
export default function GroupRemovalNotice(props: GroupMemberRemovalNoticeProps) {
    const text = useRemovalNotice(props)
    if (!text) return null
    return <Text className="text-muted-foreground text-sm">{text}</Text>
}
