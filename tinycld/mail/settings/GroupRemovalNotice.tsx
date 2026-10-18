import { and, eq, not } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import type { GroupMemberRemovalNoticeProps } from '@tinycld/core/lib/groups/removal-notice'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useRecord } from '@tinycld/core/lib/use-record'
import { materialize } from 'pbtsdb'
import { useMemo } from 'react'
import { Text } from 'react-native'
import {
    addressesLost,
    type GroupAddressRow,
    type InboxAccessRow,
    inboxesLost,
    removalNoticeText,
} from './group-removal-notice'

// The group's grants drive the query. Each grant reads its inbox and the rows
// of the person (or of every person, when the group is deleted) on that inbox,
// so only the rows of the inboxes the group grants load.
function useInboxRows({ groupId, userId }: GroupMemberRemovalNoticeProps) {
    const [membersCollection, mailboxesCollection, domainsCollection, usersCollection] = useStore(
        'mail_mailbox_members',
        'mail_mailboxes',
        'mail_domains',
        'users'
    )
    return useLiveQuery({
        queryKey: ['mail_group_removal_inboxes', groupId, userId],
        query: query =>
            query
                .from({ grant: membersCollection })
                .where(({ grant }) => and(eq(grant.group, groupId), eq(grant.user, '')))
                .select(({ grant }) => ({
                    id: grant.id,
                    mailbox: materialize(
                        query
                            .from({ mailbox: mailboxesCollection })
                            .where(({ mailbox }) => eq(mailbox.id, grant.mailbox))
                            .select(({ mailbox }) => ({
                                id: mailbox.id,
                                address: mailbox.address,
                                domain: materialize(
                                    query
                                        .from({ domain: domainsCollection })
                                        .where(({ domain }) => eq(domain.id, mailbox.domain))
                                        .findOne()
                                ),
                            }))
                            .findOne()
                    ),
                    people: materialize(
                        query
                            .from({ member: membersCollection })
                            .where(({ member }) =>
                                and(
                                    eq(member.mailbox, grant.mailbox),
                                    userId === null
                                        ? not(eq(member.user, ''))
                                        : eq(member.user, userId)
                                )
                            )
                            .select(({ member }) => ({
                                userId: member.user,
                                viaGroup: member.group,
                                user: materialize(
                                    query
                                        .from({ user: usersCollection })
                                        .where(({ user }) => eq(user.id, member.user))
                                        .findOne()
                                ),
                            }))
                    ),
                })),
    })
}

// The rule addresses the group gave. Each reads its domain (name and groups)
// and the identity of the inbox it belongs to, with the owner and the owner's
// group memberships: the memberships tell whether another of the domain's
// groups keeps the address.
function useAddressRows({ groupId, userId }: GroupMemberRemovalNoticeProps) {
    const [
        addressesCollection,
        domainsCollection,
        identitiesCollection,
        usersCollection,
        groupMembersCollection,
    ] = useStore('mail_addresses', 'mail_domains', 'mail_identities', 'users', 'group_members')
    const result = useLiveQuery({
        queryKey: ['mail_group_removal_addresses', groupId],
        query: query =>
            query
                .from({ address: addressesCollection })
                .where(({ address }) =>
                    and(eq(address.via_group, groupId), eq(address.source, 'rule'))
                )
                .select(({ address }) => ({
                    id: address.id,
                    localPart: address.local_part,
                    domain: materialize(
                        query
                            .from({ domain: domainsCollection })
                            .where(({ domain }) => eq(domain.id, address.domain))
                            .findOne()
                    ),
                    owner: materialize(
                        query
                            .from({ identity: identitiesCollection })
                            .where(({ identity }) => eq(identity.inbox, address.mailbox))
                            .select(({ identity }) => ({
                                userId: identity.user,
                                user: materialize(
                                    query
                                        .from({ user: usersCollection })
                                        .where(({ user }) => eq(user.id, identity.user))
                                        .findOne()
                                ),
                                groups: materialize(
                                    query
                                        .from({ membership: groupMembersCollection })
                                        .where(({ membership }) =>
                                            eq(membership.user, identity.user)
                                        )
                                        .select(({ membership }) => ({ group: membership.group }))
                                ),
                            }))
                            .findOne()
                    ),
                })),
    })
    // The query cannot filter by the owner, who is a related row. The rows
    // are the group's rule addresses, which the group's deletion lists anyway.
    const data = useMemo(
        () => (result.data ?? []).filter(row => userId === null || row.owner?.userId === userId),
        [result.data, userId]
    )
    return { data, isReady: result.isReady }
}

// The group's own addresses: the person stops getting them on leaving, and
// they are deleted with the group.
function useGroupAddressRows({ groupId }: GroupMemberRemovalNoticeProps) {
    const [addressesCollection, domainsCollection] = useStore('mail_addresses', 'mail_domains')
    return useLiveQuery({
        queryKey: ['mail_group_removal_group_addresses', groupId],
        query: query =>
            query
                .from({ address: addressesCollection })
                .where(({ address }) => eq(address.group, groupId))
                .select(({ address }) => ({
                    id: address.id,
                    localPart: address.local_part,
                    domain: materialize(
                        query
                            .from({ domain: domainsCollection })
                            .where(({ domain }) => eq(domain.id, address.domain))
                            .findOne()
                    ),
                })),
    })
}

// Group mail goes only to live identities, so a guest or a person with no
// inbox never got the group's addresses.
function usePersonIsLive(userId: string | null) {
    const [identitiesCollection] = useStore('mail_identities')
    const result = useLiveQuery({
        queryKey: ['mail_group_removal_identity', userId],
        query: query =>
            userId === null
                ? null
                : query
                      .from({ identity: identitiesCollection })
                      .where(({ identity }) => eq(identity.user, userId))
                      .select(({ identity }) => ({
                          retired: identity.retired,
                          inbox: identity.inbox,
                      })),
    })
    const hasLiveIdentity = (result.data ?? []).some(i => !i.retired && i.inbox !== '')
    return { hasLiveIdentity, isReady: userId === null || result.isReady }
}

function useRemovalNotice(props: GroupMemberRemovalNoticeProps) {
    const { groupId, userId } = props
    const inboxes = useInboxRows(props)
    const addresses = useAddressRows(props)
    const groupAddresses = useGroupAddressRows(props)
    const [usersCollection] = useStore('users')
    const { record: person } = useRecord(usersCollection, userId)
    const identity = usePersonIsLive(userId)
    const isReady =
        inboxes.isReady && addresses.isReady && groupAddresses.isReady && identity.isReady
    return useMemo(() => {
        // Text from one query alone would name only part of the loss.
        if (!isReady) return null
        const addressRows: GroupAddressRow[] = []
        const memberships = new Map<string, string[]>()
        const names: (string | undefined)[] = []
        for (const { localPart, domain, owner } of addresses.data) {
            if (!domain || !owner) continue
            addressRows.push({
                email: `${localPart}@${domain.domain}`,
                userId: owner.userId,
                coverageGroups: domain.coverage_groups,
            })
            memberships.set(
                owner.userId,
                owner.groups.map(m => m.group)
            )
            names.push(owner.user?.name || owner.user?.email)
        }
        const accessRows: InboxAccessRow[] = []
        for (const { mailbox, people } of inboxes.data ?? []) {
            if (!mailbox?.domain) continue
            const email = `${mailbox.address}@${mailbox.domain.domain}`
            for (const person of people) {
                accessRows.push({
                    mailboxId: mailbox.id,
                    email,
                    userId: person.userId,
                    viaGroup: person.viaGroup,
                })
                names.push(person.user?.name || person.user?.email)
            }
        }
        const personName =
            userId === null ? null : person?.name || person?.email || names.find(Boolean) || null
        // Without the name the text would read as the group's deletion.
        if (userId !== null && !personName) return null
        const getsGroupMail =
            userId === null || (identity.hasLiveIdentity && person?.role !== 'guest')
        const groupEmails = (getsGroupMail ? (groupAddresses.data ?? []) : [])
            .filter(row => row.domain)
            .map(row => `${row.localPart}@${row.domain?.domain}`)
            .sort()
        return removalNoticeText({
            personName,
            addresses: addressesLost(addressRows, memberships, groupId),
            inboxes: inboxesLost(accessRows, groupId),
            groupAddresses: groupEmails,
        })
    }, [
        isReady,
        addresses.data,
        inboxes.data,
        groupAddresses.data,
        identity.hasLiveIdentity,
        person,
        userId,
        groupId,
    ])
}

/** Contributed to core's group-member-removal slot: the mail a person or the members lose. */
export default function GroupRemovalNotice(props: GroupMemberRemovalNoticeProps) {
    const text = useRemovalNotice(props)
    if (!text) return null
    return <Text className="text-muted-foreground text-sm">{text}</Text>
}
