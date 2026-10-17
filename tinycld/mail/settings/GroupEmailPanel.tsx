import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import type { GroupDrawerPanelProps } from '@tinycld/core/lib/groups/group-panel'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Plus } from 'lucide-react-native'
import { materialize } from 'pbtsdb'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import { AddAddressForm } from './AddAddressForm'
import { domainsForGroup } from './group-domains'
import { RemoveButton } from './MailboxAddressesPanel'

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

function useGroupAddresses(groupId: string) {
    const [addressesCollection, domainsCollection] = useStore('mail_addresses', 'mail_domains')
    const { data } = useLiveQuery({
        queryKey: ['mail_group_addresses', groupId],
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
    const remove = useMutation({
        mutationFn: mutation(function* (addressId: string) {
            yield addressesCollection.delete(addressId)
        }),
    })
    const addresses = (data ?? [])
        .filter(a => a.domain)
        .map(a => ({ id: a.id, email: `${a.localPart}@${a.domain?.domain}` }))
        .sort((a, b) => a.email.localeCompare(b.email))
    return { addresses, remove }
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

function GroupAddressRow({ email, onRemove }: { email: string; onRemove: () => void }) {
    return (
        <View testID={`group-address-${email}`} className="flex-row items-center gap-3 py-1">
            <Text className="flex-1 text-foreground text-sm">{email}</Text>
            <RemoveButton isVisible email={email} onRemove={onRemove} />
        </View>
    )
}

function AddFormSlot({
    isVisible,
    groupId,
    onDone,
}: {
    isVisible: boolean
    groupId: string
    onDone: () => void
}) {
    if (!isVisible) return null
    return <AddAddressForm target={{ kind: 'group', groupId }} onDone={onDone} />
}

function NoGroupAddresses({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return <Text className="text-muted-foreground text-sm">This group has no address yet.</Text>
}

function GroupAddresses({ groupId }: { groupId: string }) {
    const primaryColor = useThemeColor('primary')
    const [isAdding, setIsAdding] = useState(false)
    const { addresses, remove } = useGroupAddresses(groupId)
    const toggleLabel = isAdding ? 'Cancel' : 'Add address'
    const rows = addresses.map(a => (
        <GroupAddressRow key={a.id} email={a.email} onRemove={() => remove.mutate(a.id)} />
    ))
    return (
        <View className="gap-2">
            <View className="flex-row items-center justify-between">
                <Text
                    className="text-muted-foreground uppercase"
                    style={{ fontSize: 11, fontWeight: '600', letterSpacing: 0.6 }}
                >
                    Group addresses
                </Text>
                <Pressable
                    onPress={() => setIsAdding(s => !s)}
                    className="flex-row gap-1 items-center"
                >
                    <Plus size={12} color={primaryColor} />
                    <Text className="text-primary" style={{ fontSize: 12, fontWeight: '600' }}>
                        {toggleLabel}
                    </Text>
                </Pressable>
            </View>
            <Text className="text-muted-foreground text-sm">
                Mail to these addresses goes to every member of the group.
            </Text>
            {rows}
            <NoGroupAddresses isVisible={addresses.length === 0} />
            <AddFormSlot isVisible={isAdding} groupId={groupId} onDone={() => setIsAdding(false)} />
        </View>
    )
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
            <GroupAddresses groupId={groupId} />
        </View>
    )
}
