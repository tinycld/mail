import { eq } from '@tanstack/db'
import { Link } from 'expo-router'
import { Lock, Mail, Plus, Trash2, UserPlus, X } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb/core'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useOrgHref } from '@tinycld/core/lib/org-routes'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useOrgLiveQuery } from '@tinycld/core/lib/use-org-live-query'
import { Divider } from '@tinycld/core/ui/divider'
import { FormErrorSummary, SelectInput, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { MailboxAliasesPanel } from './MailboxAliasesPanel'

const createMailboxSchema = z.object({
    address: z
        .string()
        .min(1, 'Address is required')
        .max(64)
        .regex(/^[a-z0-9._-]+$/, 'Only lowercase letters, numbers, dots, hyphens, underscores'),
    domain: z.string().min(1, 'Domain is required'),
    display_name: z.string().max(200).optional(),
    name: z.string().max(100).optional(),
})

interface MailboxRow {
    id: string
    address: string
    domain: string
    domainName: string
    displayName: string
    name: string
    type: string
}

interface MemberRow {
    id: string
    userOrgId: string
    userId: string
    userName: string
    role: string
}

interface OrgMemberRow {
    userOrgId: string
    userId: string
    userName: string
}

function useMailboxData() {
    const [domainsCollection, mailboxesCollection, membersCollection, userOrgCollection] = useStore(
        'mail_domains',
        'mail_mailboxes',
        'mail_mailbox_members',
        'user_org'
    )

    const { data: domains } = useOrgLiveQuery((query, { orgId }) =>
        query.from({ mail_domains: domainsCollection }).where(({ mail_domains }) => eq(mail_domains.org, orgId))
    )

    const { data: mailboxes } = useOrgLiveQuery(
        (query) =>
            query
                .from({ mail_mailboxes: mailboxesCollection })
                .where(({ mail_mailboxes }) => eq(mail_mailboxes.domain, (domains ?? [])[0]?.id ?? '')),
        [domains]
    )

    const { data: members } = useOrgLiveQuery((query) => query.from({ mail_mailbox_members: membersCollection }))

    const { data: orgUserOrgs } = useOrgLiveQuery((query, { orgId }) =>
        query.from({ user_org: userOrgCollection }).where(({ user_org }) => eq(user_org.org, orgId))
    )

    const domainMap = new Map((domains ?? []).map((d) => [d.id, d.domain]))

    const mailboxRows: MailboxRow[] = (mailboxes ?? [])
        .filter((mb) => domainMap.has(mb.domain))
        .map((mb) => ({
            id: mb.id,
            address: mb.address,
            domain: mb.domain,
            domainName: domainMap.get(mb.domain) ?? '',
            displayName: mb.display_name,
            name: mb.name,
            type: mb.type,
        }))

    const membersByMailbox = new Map<string, MemberRow[]>()
    for (const m of members ?? []) {
        const userOrg = (orgUserOrgs ?? []).find((uo) => uo.id === m.user_org)
        if (!userOrg) continue
        const row: MemberRow = {
            id: m.id,
            userOrgId: m.user_org,
            userId: userOrg.user,
            userName: userOrg.expand?.user?.name || userOrg.expand?.user?.email || userOrg.user,
            role: m.role,
        }
        const list = membersByMailbox.get(m.mailbox) ?? []
        list.push(row)
        membersByMailbox.set(m.mailbox, list)
    }

    const orgMembers: OrgMemberRow[] = (orgUserOrgs ?? []).map((uo) => ({
        userOrgId: uo.id,
        userId: uo.user,
        userName: uo.expand?.user?.name || uo.expand?.user?.email || uo.user,
    }))

    return {
        domains: domains ?? [],
        mailboxRows,
        membersByMailbox,
        orgMembers,
    }
}

export default function MailboxesSettings() {
    const foregroundColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const primaryColor = useThemeColor('primary')
    const backgroundColor = useThemeColor('background')
    const { isAdmin, userOrgId } = useCurrentRole()
    const [expandedMailbox, setExpandedMailbox] = useState<string | null>(null)
    const data = useMailboxData()

    if (!isAdmin) {
        return (
            <View className="flex-1 items-center justify-center p-5">
                <Text style={{ color: mutedColor }}>Admin access required</Text>
            </View>
        )
    }

    const domainOptions = data.domains.map((d) => ({ label: d.domain, value: d.id }))
    const hasDomains = data.domains.length > 0

    return (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} style={{ backgroundColor }}>
            <View className="flex-1 gap-5 p-5" style={{ maxWidth: 600 }}>
                <View className="gap-2">
                    <Mail size={32} color={primaryColor} />
                    <Text style={{ fontSize: 20, fontWeight: 'bold', color: foregroundColor }}>Mailboxes</Text>
                    <Text style={{ fontSize: 13, color: mutedColor }}>
                        Manage mailboxes and member assignments for your organization.
                    </Text>
                </View>

                <NoDomainsBanner isVisible={!hasDomains} />

                {hasDomains && (
                    <>
                        <View className="gap-3">
                            <Text style={{ fontSize: 18, fontWeight: 'bold', color: foregroundColor }}>Mailboxes</Text>
                            <NoMailboxesBanner isVisible={data.mailboxRows.length === 0} />
                            {data.mailboxRows.map((mb) => (
                                <MailboxCard
                                    key={mb.id}
                                    mailbox={mb}
                                    members={data.membersByMailbox.get(mb.id) ?? []}
                                    orgMembers={data.orgMembers}
                                    isExpanded={expandedMailbox === mb.id}
                                    onToggle={() => setExpandedMailbox((prev) => (prev === mb.id ? null : mb.id))}
                                />
                            ))}
                        </View>

                        <Divider />

                        <CreateMailboxForm domainOptions={domainOptions} userOrgId={userOrgId} />
                    </>
                )}
            </View>
        </ScrollView>
    )
}

function NoDomainsBanner({ isVisible }: { isVisible: boolean }) {
    const mutedColor = useThemeColor('muted-foreground')
    const accentColor = useThemeColor('primary')
    const orgHref = useOrgHref()

    if (!isVisible) return null
    return (
        <Text style={{ color: mutedColor }}>
            No mail domains configured.{' '}
            <Link href={orgHref('settings/[...section]', { section: ['mail', 'provider'] })}>
                <Text style={{ color: accentColor, textDecorationLine: 'underline' }}>
                    Add a domain in Provider settings
                </Text>
            </Link>{' '}
            first.
        </Text>
    )
}

function NoMailboxesBanner({ isVisible }: { isVisible: boolean }) {
    const mutedColor = useThemeColor('muted-foreground')
    if (!isVisible) return null
    return <Text style={{ color: mutedColor }}>No mailboxes yet.</Text>
}

function MailboxCard({
    mailbox,
    members,
    orgMembers,
    isExpanded,
    onToggle,
}: {
    mailbox: MailboxRow
    members: MemberRow[]
    orgMembers: OrgMemberRow[]
    isExpanded: boolean
    onToggle: () => void
}) {
    const foregroundColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const borderColor = useThemeColor('border')
    const isPersonal = mailbox.type === 'personal'
    const fullAddress = `${mailbox.address}@${mailbox.domainName}`

    return (
        <View className="border rounded-xl p-3" style={{ borderColor }}>
            <Pressable className="flex-row justify-between items-center" onPress={onToggle}>
                <View className="gap-1 flex-1">
                    <View className="flex-row gap-2 items-center">
                        {isPersonal && <Lock size={14} color={mutedColor} />}
                        <Text style={{ fontWeight: '600', color: foregroundColor }}>{fullAddress}</Text>
                        <TypeBadge type={mailbox.type} />
                    </View>
                    <Text style={{ fontSize: 13, color: mutedColor }}>
                        {mailbox.name ? `${mailbox.name} \u00b7 ` : ''}
                        {mailbox.displayName || 'No display name'} \u00b7 {members.length} member
                        {members.length !== 1 ? 's' : ''}
                    </Text>
                </View>

                <DeleteMailboxButton mailboxId={mailbox.id} isVisible={!isPersonal} />
            </Pressable>

            <MailboxMemberPanel
                isVisible={isExpanded}
                mailboxId={mailbox.id}
                members={members}
                orgMembers={orgMembers}
            />

            {isExpanded && (
                <MailboxAliasesPanel
                    mailboxId={mailbox.id}
                    mailboxDomainId={mailbox.domain}
                    domainName={mailbox.domainName}
                />
            )}
        </View>
    )
}

function TypeBadge({ type }: { type: string }) {
    const isPersonal = type === 'personal'
    return (
        <View className={`px-2 py-0.5 rounded-lg ${isPersonal ? 'bg-[#dbeafe]' : 'bg-[#dcfce7]'}`}>
            <Text className={`text-[11px] ${isPersonal ? 'text-[#2563eb]' : 'text-[#16a34a]'}`}>{type}</Text>
        </View>
    )
}

function DeleteMailboxButton({ mailboxId, isVisible }: { mailboxId: string; isVisible: boolean }) {
    const dangerColor = useThemeColor('danger')
    const _primaryColor = useThemeColor('primary')
    const [mailboxesCollection] = useStore('mail_mailboxes')
    const [confirming, setConfirming] = useState(false)

    const deleteMutation = useMutation({
        mutationFn: mutation(function* () {
            yield mailboxesCollection.delete(mailboxId)
        }),
        onSuccess: () => setConfirming(false),
    })

    if (!isVisible) return null

    if (confirming) {
        return (
            <View className="flex-row gap-2">
                <Pressable
                    onPress={() => deleteMutation.mutate()}
                    className="px-3 rounded-md"
                    style={{ paddingVertical: 6, backgroundColor: dangerColor }}
                >
                    <Text style={{ fontSize: 13, color: '#fff' }}>Confirm</Text>
                </Pressable>
                <Pressable
                    onPress={() => setConfirming(false)}
                    className="px-3 rounded-md"
                    style={{ paddingVertical: 6 }}
                >
                    <Text style={{ fontSize: 13 }}>Cancel</Text>
                </Pressable>
            </View>
        )
    }

    return (
        <Pressable
            className="p-1"
            onPress={(e) => {
                e.stopPropagation()
                setConfirming(true)
            }}
        >
            <Trash2 size={16} color={dangerColor} />
        </Pressable>
    )
}

function MailboxMemberPanel({
    isVisible,
    mailboxId,
    members,
    orgMembers,
}: {
    isVisible: boolean
    mailboxId: string
    members: MemberRow[]
    orgMembers: OrgMemberRow[]
}) {
    const foregroundColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const borderColor = useThemeColor('border')
    const dangerColor = useThemeColor('danger')
    const [membersCollection] = useStore('mail_mailbox_members')
    const [addingMember, setAddingMember] = useState(false)
    const [selectedUserOrg, setSelectedUserOrg] = useState('')

    const existingUserOrgIds = new Set(members.map((m) => m.userOrgId))
    const availableMembers = orgMembers.filter((uo) => !existingUserOrgIds.has(uo.userOrgId))

    const addMemberMutation = useMutation({
        mutationFn: mutation(function* () {
            yield membersCollection.insert({
                id: newRecordId(),
                mailbox: mailboxId,
                user_org: selectedUserOrg,
                role: 'member',
            })
        }),
        onSuccess: () => {
            setAddingMember(false)
            setSelectedUserOrg('')
        },
    })

    const removeMemberMutation = useMutation({
        mutationFn: mutation(function* (memberId: string) {
            yield membersCollection.delete(memberId)
        }),
    })

    const toggleRoleMutation = useMutation({
        mutationFn: mutation(function* ({ memberId, newRole }: { memberId: string; newRole: string }) {
            yield membersCollection.update(memberId, (draft) => {
                draft.role = newRole as 'owner' | 'member'
            })
        }),
    })

    if (!isVisible) return null

    const ownerCount = members.filter((m) => m.role === 'owner').length

    return (
        <View
            className="gap-2 mt-3 pt-3"
            style={{
                borderTopWidth: 1,
                borderColor,
            }}
        >
            <Text style={{ fontSize: 13, fontWeight: '600', color: foregroundColor }}>Members</Text>

            {members.map((m) => {
                const isOwner = m.role === 'owner'
                const canRemove = !(isOwner && ownerCount <= 1)

                return (
                    <View key={m.id} className="flex-row justify-between items-center py-1">
                        <View className="flex-1">
                            <Text style={{ fontSize: 13, color: foregroundColor }}>{m.userName}</Text>
                            <Text style={{ fontSize: 11, color: mutedColor }}>{m.role}</Text>
                        </View>

                        <View className="flex-row gap-2">
                            <Pressable
                                onPress={() =>
                                    toggleRoleMutation.mutate({
                                        memberId: m.id,
                                        newRole: isOwner ? 'member' : 'owner',
                                    })
                                }
                                disabled={isOwner && ownerCount <= 1}
                                className={`p-1 ${isOwner && ownerCount <= 1 ? 'opacity-40' : 'opacity-100'}`}
                            >
                                <Text style={{ fontSize: 13 }}>{isOwner ? 'Make member' : 'Make owner'}</Text>
                            </Pressable>

                            <Pressable
                                onPress={() => removeMemberMutation.mutate(m.id)}
                                disabled={!canRemove}
                                className={`p-1 ${canRemove ? 'opacity-100' : 'opacity-40'}`}
                            >
                                <X size={14} color={dangerColor} />
                            </Pressable>
                        </View>
                    </View>
                )
            })}

            <AddMemberSection
                isVisible={addingMember}
                availableMembers={availableMembers}
                selectedUserOrg={selectedUserOrg}
                onSelect={setSelectedUserOrg}
                onAdd={() => addMemberMutation.mutate()}
                onCancel={() => {
                    setAddingMember(false)
                    setSelectedUserOrg('')
                }}
                isPending={addMemberMutation.isPending}
            />

            <AddMemberTrigger
                isVisible={!addingMember}
                onPress={() => setAddingMember(true)}
                disabled={availableMembers.length === 0}
            />
        </View>
    )
}

function AddMemberSection({
    isVisible,
    availableMembers,
    selectedUserOrg,
    onSelect,
    onAdd,
    onCancel,
    isPending,
}: {
    isVisible: boolean
    availableMembers: OrgMemberRow[]
    selectedUserOrg: string
    onSelect: (id: string) => void
    onAdd: () => void
    onCancel: () => void
    isPending: boolean
}) {
    const mutedColor = useThemeColor('muted-foreground')
    const accentColor = useThemeColor('primary')
    const primaryColor = useThemeColor('primary')
    const primaryFgColor = useThemeColor('primary-foreground')
    const borderColor = useThemeColor('border')

    if (!isVisible) return null

    return (
        <View className="gap-2">
            <Text style={{ fontSize: 13, color: mutedColor }}>Select a member to add:</Text>
            <View className="flex-row gap-1 flex-wrap">
                {availableMembers.map((uo) => {
                    const isSelected = selectedUserOrg === uo.userOrgId
                    return (
                        <Pressable
                            key={uo.userOrgId}
                            onPress={() => onSelect(uo.userOrgId)}
                            className="border rounded-xl px-3"
                            style={{
                                borderColor: isSelected ? accentColor : borderColor,
                                paddingVertical: 6,
                                backgroundColor: isSelected ? `${accentColor}14` : undefined,
                            }}
                        >
                            <Text style={{ fontSize: 13 }}>{uo.userName}</Text>
                        </Pressable>
                    )
                })}
            </View>
            <View className="flex-row gap-2">
                <Pressable
                    onPress={onAdd}
                    disabled={!selectedUserOrg || isPending}
                    className={`px-4 rounded-lg py-2.5 ${!selectedUserOrg || isPending ? 'opacity-50' : 'opacity-100'}`}
                    style={{ backgroundColor: primaryColor }}
                >
                    <Text style={{ color: primaryFgColor }}>Add</Text>
                </Pressable>
                <Pressable onPress={onCancel} className="px-4 rounded-lg" style={{ paddingVertical: 10 }}>
                    <Text>Cancel</Text>
                </Pressable>
            </View>
        </View>
    )
}

function AddMemberTrigger({
    isVisible,
    onPress,
    disabled,
}: {
    isVisible: boolean
    onPress: () => void
    disabled: boolean
}) {
    if (!isVisible) return null

    return (
        <Pressable
            onPress={onPress}
            disabled={disabled}
            className={`flex-row gap-1 items-center p-1 ${disabled ? 'opacity-40' : 'opacity-100'}`}
        >
            <UserPlus size={14} />
            <Text style={{ fontSize: 13 }}>Add member</Text>
        </Pressable>
    )
}

function CreateMailboxForm({
    domainOptions,
    userOrgId,
}: {
    domainOptions: { label: string; value: string }[]
    userOrgId: string
}) {
    const foregroundColor = useThemeColor('foreground')
    const primaryColor = useThemeColor('primary')
    const primaryFgColor = useThemeColor('primary-foreground')
    const [mailboxesCollection, membersCollection] = useStore('mail_mailboxes', 'mail_mailbox_members')

    const {
        control,
        handleSubmit,
        setError,
        getValues,
        reset,
        formState: { errors, isSubmitted, isDirty },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(createMailboxSchema),
        defaultValues: {
            address: '',
            domain: domainOptions[0]?.value ?? '',
            display_name: '',
            name: '',
        },
    })

    const createMutation = useMutation({
        mutationFn: mutation(function* (data: z.infer<typeof createMailboxSchema>) {
            const mailboxId = newRecordId()
            yield mailboxesCollection.insert({
                id: mailboxId,
                address: data.address,
                domain: data.domain,
                display_name: data.display_name ?? '',
                name: data.name ?? '',
                type: 'shared',
            })
            yield membersCollection.insert({
                id: newRecordId(),
                mailbox: mailboxId,
                user_org: userOrgId,
                role: 'owner',
            })
        }),
        onSuccess: () => reset(),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const onSubmit = handleSubmit((data) => createMutation.mutate(data))
    const canSubmit = !createMutation.isPending && isDirty

    return (
        <View className="gap-3">
            <Text style={{ fontSize: 18, fontWeight: 'bold', color: foregroundColor }}>Create Shared Mailbox</Text>

            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

            <TextInput control={control} name="address" label="Address" placeholder="support" />

            <SelectInput control={control} name="domain" label="Domain" options={domainOptions} />

            <TextInput control={control} name="display_name" label="Display Name" placeholder="Support Team" />

            <TextInput control={control} name="name" label="Mailbox Name" placeholder="Acme Corp" />

            <Pressable
                onPress={onSubmit}
                disabled={!canSubmit}
                className={`flex-row items-center justify-center gap-2 rounded-lg h-11 ${canSubmit ? 'opacity-100' : 'opacity-50'}`}
                style={{ backgroundColor: primaryColor }}
            >
                <Plus size={16} color={primaryFgColor} />
                <Text style={{ fontWeight: '600', color: primaryFgColor }}>
                    {createMutation.isPending ? 'Creating...' : 'Create Mailbox'}
                </Text>
            </Pressable>
        </View>
    )
}
