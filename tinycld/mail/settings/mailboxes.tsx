import { and, eq, not } from '@tanstack/db'
import { HelpIcon } from '@tinycld/core/components/help/HelpIcon'
import { useAuth } from '@tinycld/core/lib/auth'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { Link } from 'expo-router'
import { Plus } from 'lucide-react-native'
import { useMemo, useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import {
    filterAndGroupMailboxes,
    type MailboxListItem,
    type TypeFilter,
} from '../hooks/filterMailboxes'
import { type DrawerMode, MailboxDrawer } from './MailboxDrawer'
import { MailboxListRow } from './MailboxListRow'
import { MailboxSearchBar } from './MailboxSearchBar'

interface MemberRow {
    id: string
    userId: string
    userName: string
    userEmail: string
    role: 'owner' | 'member'
    isYou: boolean
}

interface OrgMemberRow {
    userId: string
    userName: string
    userEmail: string
}

function useMailboxData(currentUserId: string) {
    const [
        domainsCollection,
        mailboxesCollection,
        membersCollection,
        aliasesCollection,
        usersCollection,
    ] = useStore(
        'mail_domains',
        'mail_mailboxes',
        'mail_mailbox_members',
        'mail_mailbox_aliases',
        'users'
    )

    // Standalone: gates the whole screen and feeds the create-form's domain
    // options, both of which must work with zero mailboxes.
    const { data: domains } = useOrgLiveQuery(query =>
        query.from({ mail_domains: domainsCollection })
    )

    // Each mailbox resolves its domain in the same expression; one whose
    // domain is gone drops out via the inner join.
    const { data: mailboxRows } = useOrgLiveQuery(query =>
        query
            .from({ mailbox: mailboxesCollection })
            .innerJoin({ domain: domainsCollection }, ({ mailbox, domain }) =>
                eq(mailbox.domain, domain.id)
            )
    )

    // Membership rows carry their user in the same expression; a row whose
    // user is gone drops out via the inner join.
    const { data: memberRows } = useOrgLiveQuery(query =>
        query
            .from({ member: membersCollection })
            .innerJoin({ user: usersCollection }, ({ member, user }) => eq(member.user, user.id))
    )

    const { data: aliases } = useOrgLiveQuery(query =>
        query.from({ mail_mailbox_aliases: aliasesCollection })
    )

    // Single-org: every real member is a potential mailbox member — the
    // add-member picker needs users with no membership row, so this cannot
    // derive from the membership join. Guests and disabled accounts are
    // excluded: guests never get mail infra (see the 1830000002/3 rules) and a
    // suspended account must not be granted new access.
    const { data: orgUsers } = useOrgLiveQuery(query =>
        query
            .from({ users: usersCollection })
            .where(({ users }) => and(not(eq(users.role, 'guest')), not(eq(users.disabled, true))))
    )

    const membersByMailbox = new Map<string, MemberRow[]>()
    for (const { member, user } of memberRows ?? []) {
        const row: MemberRow = {
            id: member.id,
            userId: user.id,
            userName: user.name || user.email || user.id,
            userEmail: user.email ?? '',
            role: member.role as 'owner' | 'member',
            isYou: user.id === currentUserId,
        }
        const list = membersByMailbox.get(member.mailbox) ?? []
        list.push(row)
        membersByMailbox.set(member.mailbox, list)
    }

    const aliasesByMailbox = new Map<string, string[]>()
    for (const a of aliases ?? []) {
        const list = aliasesByMailbox.get(a.mailbox) ?? []
        list.push(a.address)
        aliasesByMailbox.set(a.mailbox, list)
    }

    const items: MailboxListItem[] = (mailboxRows ?? []).map(({ mailbox, domain }) => {
        const mbMembers = membersByMailbox.get(mailbox.id) ?? []
        const mbAliases = aliasesByMailbox.get(mailbox.id) ?? []
        return {
            id: mailbox.id,
            address: mailbox.address,
            domainName: domain.domain,
            displayName: mailbox.display_name || mailbox.name || '',
            type: mailbox.type,
            memberCount: mbMembers.length,
            aliasCount: mbAliases.length,
            memberNames: mbMembers.map(m => m.userName),
            memberEmails: mbMembers.map(m => m.userEmail).filter(Boolean),
            aliasAddresses: mbAliases,
        }
    })

    const orgMembers: OrgMemberRow[] = (orgUsers ?? []).map(u => ({
        userId: u.id,
        userName: u.name || u.email || u.id,
        userEmail: u.email ?? '',
    }))

    const rawMailboxes = new Map((mailboxRows ?? []).map(r => [r.mailbox.id, r.mailbox]))

    return {
        domains: domains ?? [],
        items,
        rawMailboxes,
        membersByMailbox,
        orgMembers,
    }
}

export default function MailboxesSettings() {
    const primaryColor = useThemeColor('primary')
    const primaryFgColor = useThemeColor('primary-foreground')

    const { isAdmin } = useCurrentRole()
    const currentUserId = useAuth().user.id
    const data = useMailboxData(currentUserId)
    const orgHref = useOrgHref()

    const [query, setQuery] = useState('')
    const [typeFilter, setTypeFilter] = useState<TypeFilter>('all')
    const [drawerMode, setDrawerMode] = useState<DrawerMode>({ kind: 'closed' })

    const grouped = useMemo(
        () => filterAndGroupMailboxes(data.items, { query, type: typeFilter }),
        [data.items, query, typeFilter]
    )

    const selectedMailboxId =
        drawerMode.kind === 'view' || drawerMode.kind === 'edit' ? drawerMode.mailboxId : null

    const selectedMailbox = useMemo(() => {
        if (!selectedMailboxId) return null
        const raw = data.rawMailboxes.get(selectedMailboxId)
        if (!raw) return null
        const domainName = data.domains.find(d => d.id === raw.domain)?.domain ?? ''
        return {
            id: raw.id,
            address: raw.address,
            domain: raw.domain,
            domainName,
            displayName: raw.display_name || raw.name || '',
            type: raw.type as 'shared' | 'personal',
            createdAt: raw.created ?? '',
        }
    }, [data.domains, data.rawMailboxes, selectedMailboxId])

    const selectedMembers = selectedMailboxId
        ? (data.membersByMailbox.get(selectedMailboxId) ?? [])
        : []

    if (!isAdmin) {
        return (
            <View className="flex-1 items-center justify-center p-5 bg-background">
                <Text className="text-muted-foreground">Admin access required</Text>
            </View>
        )
    }

    const domainOptions = data.domains.map(d => ({ label: d.domain, value: d.id }))
    const hasDomains = data.domains.length > 0

    return (
        <>
            <ScrollView contentContainerStyle={{ flexGrow: 1 }} className="bg-background">
                <View className="flex-1 gap-5 p-5">
                    <View className="flex-row items-start justify-between gap-4 flex-wrap">
                        <View className="gap-1 flex-1" style={{ minWidth: 220 }}>
                            <Text className="text-muted-foreground" style={{ fontSize: 11 }}>
                                Settings · Mail
                            </Text>
                            <View className="flex-row items-center gap-2">
                                <Text
                                    className="text-foreground"
                                    style={{ fontSize: 22, fontWeight: '700' }}
                                >
                                    Mailboxes
                                </Text>
                                <HelpIcon topic="mail:mailboxes" size={18} />
                            </View>
                            <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                                Manage shared mailboxes, members, and aliases for your organization.
                            </Text>
                        </View>
                        <Pressable
                            onPress={() => setDrawerMode({ kind: 'create' })}
                            disabled={!hasDomains}
                            className="flex-row gap-1.5 items-center rounded-lg"
                            style={{
                                paddingVertical: 8,
                                paddingHorizontal: 14,
                                backgroundColor: primaryColor,
                                opacity: hasDomains ? 1 : 0.4,
                            }}
                        >
                            <Plus size={14} color={primaryFgColor} />
                            <Text
                                style={{ color: primaryFgColor, fontSize: 13, fontWeight: '600' }}
                            >
                                New mailbox
                            </Text>
                        </Pressable>
                    </View>

                    {!hasDomains && (
                        <Text className="text-muted-foreground">
                            No mail domains configured.{' '}
                            <Link
                                href={orgHref('settings/[...section]', {
                                    section: ['mail', 'provider'],
                                })}
                            >
                                <Text
                                    className="text-primary"
                                    style={{ textDecorationLine: 'underline' }}
                                >
                                    Add a domain in Provider settings
                                </Text>
                            </Link>{' '}
                            first.
                        </Text>
                    )}

                    {hasDomains && (
                        <>
                            <MailboxSearchBar
                                query={query}
                                onQueryChange={setQuery}
                                type={typeFilter}
                                onTypeChange={setTypeFilter}
                                counts={grouped.totals}
                            />

                            {data.items.length === 0 && (
                                <View className="items-center gap-2 p-6">
                                    <Text className="text-muted-foreground">No mailboxes yet.</Text>
                                    <Pressable
                                        onPress={() => setDrawerMode({ kind: 'create' })}
                                        className="rounded-lg"
                                        style={{
                                            paddingVertical: 8,
                                            paddingHorizontal: 14,
                                            backgroundColor: primaryColor,
                                        }}
                                    >
                                        <Text
                                            style={{
                                                color: primaryFgColor,
                                                fontSize: 13,
                                                fontWeight: '600',
                                            }}
                                        >
                                            Create your first mailbox
                                        </Text>
                                    </Pressable>
                                </View>
                            )}

                            {data.items.length > 0 &&
                                grouped.shared.length === 0 &&
                                grouped.personal.length === 0 && (
                                    <View className="items-center gap-2 p-6">
                                        <Text className="text-muted-foreground">
                                            No mailboxes match "{query}".
                                        </Text>
                                        <Pressable onPress={() => setQuery('')}>
                                            <Text
                                                className="text-primary"
                                                style={{ fontSize: 13, fontWeight: '600' }}
                                            >
                                                Clear search
                                            </Text>
                                        </Pressable>
                                    </View>
                                )}

                            {grouped.shared.length > 0 && (
                                <View className="gap-1">
                                    <GroupLabel label="Shared" count={grouped.shared.length} />
                                    {grouped.shared.map(item => (
                                        <MailboxListRow
                                            key={item.id}
                                            item={item}
                                            isActive={selectedMailboxId === item.id}
                                            onPress={() =>
                                                setDrawerMode({ kind: 'view', mailboxId: item.id })
                                            }
                                        />
                                    ))}
                                </View>
                            )}

                            {grouped.personal.length > 0 && (
                                <View className="gap-1">
                                    <GroupLabel label="Personal" count={grouped.personal.length} />
                                    {grouped.personal.map(item => (
                                        <MailboxListRow
                                            key={item.id}
                                            item={item}
                                            isActive={selectedMailboxId === item.id}
                                            onPress={() =>
                                                setDrawerMode({ kind: 'view', mailboxId: item.id })
                                            }
                                        />
                                    ))}
                                </View>
                            )}

                            {data.items.length > 0 && (
                                <View className="flex-row items-center justify-between pt-3 border-t border-border">
                                    <Text
                                        className="text-muted-foreground"
                                        style={{ fontSize: 12 }}
                                    >
                                        {data.items.length} mailbox
                                        {data.items.length !== 1 ? 'es' : ''}
                                        {data.domains[0]?.domain
                                            ? ` · ${data.domains[0].domain}`
                                            : ''}
                                    </Text>
                                    <Link
                                        href={orgHref('settings/[...section]', {
                                            section: ['mail', 'provider'],
                                        })}
                                    >
                                        <Text
                                            className="text-primary"
                                            style={{ fontSize: 12, fontWeight: '600' }}
                                        >
                                            Manage domains ›
                                        </Text>
                                    </Link>
                                </View>
                            )}
                        </>
                    )}
                </View>
            </ScrollView>

            <MailboxDrawer
                mode={drawerMode}
                onClose={() => setDrawerMode({ kind: 'closed' })}
                onSwitchToEdit={id => setDrawerMode({ kind: 'edit', mailboxId: id })}
                onSwitchToView={id => setDrawerMode({ kind: 'view', mailboxId: id })}
                mailbox={selectedMailbox}
                members={selectedMembers}
                orgMembers={data.orgMembers}
                domainOptions={domainOptions}
                userId={currentUserId}
            />
        </>
    )
}

function GroupLabel({ label, count }: { label: string; count: number }) {
    return (
        <View className="flex-row items-center gap-2 pt-3 pb-1 px-2">
            <Text
                className="text-muted-foreground"
                style={{
                    fontSize: 10.5,
                    fontWeight: '600',
                    textTransform: 'uppercase',
                    letterSpacing: 0.8,
                }}
            >
                {label}
            </Text>
            <Text
                className="text-muted-foreground"
                style={{
                    fontSize: 10.5,
                    fontWeight: '600',
                }}
            >
                {count}
            </Text>
        </View>
    )
}
