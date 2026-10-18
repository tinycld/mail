import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import type { GroupDrawerPanelProps } from '@tinycld/core/lib/groups/group-panel'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { Text, View } from 'react-native'
import { domainsForGroup } from './group-domains'

function useGroupDomains(groupId: string) {
    const [domainsCollection] = useStore('mail_domains')
    const { data, isReady } = useLiveQuery({
        queryKey: ['mail_group_domains'],
        query: query =>
            query
                .from({ domain: domainsCollection })
                .where(({ domain }) => eq(domain.coverage, 'groups'))
                .select(({ domain }) => ({
                    id: domain.id,
                    domain: domain.domain,
                    coverageGroups: domain.coverage_groups,
                    rank: domain.rank,
                })),
    })
    return { domains: domainsForGroup(data ?? [], groupId), isReady }
}

function useGroupInboxes(groupId: string) {
    const [membersCollection, mailboxesCollection, domainsCollection] = useStore(
        'mail_mailbox_members',
        'mail_mailboxes',
        'mail_domains'
    )
    const { data } = useLiveQuery({
        queryKey: ['mail_group_inboxes', groupId],
        query: query =>
            query
                .from({ grant: membersCollection })
                .innerJoin({ mailbox: mailboxesCollection }, ({ grant, mailbox }) =>
                    eq(grant.mailbox, mailbox.id)
                )
                .innerJoin({ domain: domainsCollection }, ({ mailbox, domain }) =>
                    eq(mailbox.domain, domain.id)
                )
                .where(({ grant }) => and(eq(grant.group, groupId), eq(grant.user, '')))
                .select(({ mailbox, domain }) => ({
                    id: mailbox.id,
                    address: mailbox.address,
                    domainName: domain.domain,
                })),
    })
    return (data ?? []).map(m => ({ id: m.id, email: `${m.address}@${m.domainName}` }))
}

function InboxLine({ email }: { email: string }) {
    return <Text className="text-foreground text-sm">Can use {email}</Text>
}

function DomainLine({ domain }: { domain: string }) {
    return <Text className="text-foreground text-sm">Gives addresses on {domain}</Text>
}

function NoDomains({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return <Text className="text-muted-foreground text-sm">This group gives no addresses.</Text>
}

/** Contributed to core's group drawer: the domains that give this group's members addresses. */
export default function GroupEmailPanel({ groupId }: GroupDrawerPanelProps) {
    const { domains, isReady } = useGroupDomains(groupId)
    const inboxes = useGroupInboxes(groupId)
    const inboxLines = inboxes.map(i => <InboxLine key={i.id} email={i.email} />)
    const lines = domains.map(d => <DomainLine key={d.id} domain={d.domain} />)
    return (
        <View className="gap-2">
            <Text className="text-[12px] font-semibold text-muted uppercase tracking-wide">
                Email
            </Text>
            {lines}
            {inboxLines}
            <NoDomains isVisible={isReady && domains.length === 0} />
        </View>
    )
}
