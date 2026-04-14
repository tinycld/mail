import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { Link } from 'expo-router'
import { Lock, Mail, Plus, Trash2, UserPlus, X } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb'
import { useState } from 'react'
import { Pressable, ScrollView, Text, View } from 'react-native'
import { handleMutationErrorsWithForm } from '~/lib/errors'
import { mutation, useMutation } from '~/lib/mutations'
import { useOrgHref } from '~/lib/org-routes'
import { useStore } from '~/lib/pocketbase'
import { useThemeColor } from '~/lib/use-app-theme'
import { useCurrentRole } from '~/lib/use-current-role'
import { useOrgInfo } from '~/lib/use-org-info'
import { Divider } from '~/ui/divider'
import { FormErrorSummary, SelectInput, TextInput, useForm, z, zodResolver } from '~/ui/form'

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

function useMailboxData(orgId: string) {
    const [domainsCollection, mailboxesCollection, membersCollection, userOrgCollection] = useStore(
        'mail_domains',
        'mail_mailboxes',
        'mail_mailbox_members',
        'user_org'
    )

    const { data: domains } = useLiveQuery(
        query =>
            query
                .from({ mail_domains: domainsCollection })
                .where(({ mail_domains }) => eq(mail_domains.org, orgId)),
        [orgId]
    )

    const { data: mailboxes } = useLiveQuery(
        query =>
            query
                .from({ mail_mailboxes: mailboxesCollection })
                .where(({ mail_mailboxes }) =>
                    eq(mail_mailboxes.domain, (domains ?? [])[0]?.id ?? '')
                ),
        [domains]
    )

    const { data: members } = useLiveQuery(
        query => query.from({ mail_mailbox_members: membersCollection }),
        []
    )

    const { data: orgUserOrgs } = useLiveQuery(
        query =>
            query
                .from({ user_org: userOrgCollection })
                .where(({ user_org }) => eq(user_org.org, orgId)),
        [orgId]
    )

    const domainMap = new Map((domains ?? []).map(d => [d.id, d.domain]))

    const mailboxRows: MailboxRow[] = (mailboxes ?? [])
        .filter(mb => domainMap.has(mb.domain))
        .map(mb => ({
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
        const userOrg = (orgUserOrgs ?? []).find(uo => uo.id === m.user_org)
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

    const orgMembers: OrgMemberRow[] = (orgUserOrgs ?? []).map(uo => ({
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
    const { orgId } = useOrgInfo()
    const { isAdmin, userOrgId } = useCurrentRole()
    const [expandedMailbox, setExpandedMailbox] = useState<string | null>(null)
    const data = useMailboxData(orgId)

    if (!isAdmin) {
        return (
            <View style={{ flex: 1, padding: 20, justifyContent: 'center', alignItems: 'center' }}>
                <Text style={{ color: mutedColor }}>Admin access required</Text>
            </View>
        )
    }

    const domainOptions = data.domains.map(d => ({ label: d.domain, value: d.id }))
    const hasDomains = data.domains.length > 0

    return (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} style={{ backgroundColor }}>
            <View style={{ flex: 1, padding: 20, gap: 20, maxWidth: 600 }}>
                <View style={{ gap: 8 }}>
                    <Mail size={32} color={primaryColor} />
                    <Text style={{ fontSize: 20, fontWeight: 'bold', color: foregroundColor }}>
                        Mailboxes
                    </Text>
                    <Text style={{ fontSize: 13, color: mutedColor }}>
                        Manage mailboxes and member assignments for your organization.
                    </Text>
                </View>

                <NoDomainsBanner isVisible={!hasDomains} />

                {hasDomains && (
                    <>
                        <View style={{ gap: 12 }}>
                            <Text
                                style={{ fontSize: 18, fontWeight: 'bold', color: foregroundColor }}
                            >
                                Mailboxes
                            </Text>
                            <NoMailboxesBanner isVisible={data.mailboxRows.length === 0} />
                            {data.mailboxRows.map(mb => (
                                <MailboxCard
                                    key={mb.id}
                                    mailbox={mb}
                                    members={data.membersByMailbox.get(mb.id) ?? []}
                                    orgMembers={data.orgMembers}
                                    isExpanded={expandedMailbox === mb.id}
                                    onToggle={() =>
                                        setExpandedMailbox(prev => (prev === mb.id ? null : mb.id))
                                    }
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
    const accentColor = useThemeColor('accent')
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
        <View style={{ borderWidth: 1, borderColor, borderRadius: 12, padding: 12 }}>
            <Pressable
                style={{
                    flexDirection: 'row',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                }}
                onPress={onToggle}
            >
                <View style={{ gap: 4, flex: 1 }}>
                    <View style={{ flexDirection: 'row', gap: 8, alignItems: 'center' }}>
                        {isPersonal && <Lock size={14} color={mutedColor} />}
                        <Text style={{ fontWeight: '600', color: foregroundColor }}>
                            {fullAddress}
                        </Text>
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
        </View>
    )
}

function TypeBadge({ type }: { type: string }) {
    const isPersonal = type === 'personal'
    return (
        <View
            style={{
                backgroundColor: isPersonal ? '#dbeafe' : '#dcfce7',
                paddingHorizontal: 8,
                paddingVertical: 2,
                borderRadius: 8,
            }}
        >
            <Text style={{ fontSize: 11, color: isPersonal ? '#2563eb' : '#16a34a' }}>{type}</Text>
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
            <View style={{ flexDirection: 'row', gap: 8 }}>
                <Pressable
                    onPress={() => deleteMutation.mutate()}
                    style={{
                        paddingHorizontal: 12,
                        paddingVertical: 6,
                        borderRadius: 6,
                        backgroundColor: dangerColor,
                    }}
                >
                    <Text style={{ fontSize: 13, color: '#fff' }}>Confirm</Text>
                </Pressable>
                <Pressable
                    onPress={() => setConfirming(false)}
                    style={{ paddingHorizontal: 12, paddingVertical: 6, borderRadius: 6 }}
                >
                    <Text style={{ fontSize: 13 }}>Cancel</Text>
                </Pressable>
            </View>
        )
    }

    return (
        <Pressable
            style={{ padding: 4 }}
            onPress={e => {
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

    const existingUserOrgIds = new Set(members.map(m => m.userOrgId))
    const availableMembers = orgMembers.filter(uo => !existingUserOrgIds.has(uo.userOrgId))

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
        mutationFn: mutation(function* ({
            memberId,
            newRole,
        }: {
            memberId: string
            newRole: string
        }) {
            yield membersCollection.update(memberId, draft => {
                draft.role = newRole as 'owner' | 'member'
            })
        }),
    })

    if (!isVisible) return null

    const ownerCount = members.filter(m => m.role === 'owner').length

    return (
        <View
            style={{
                gap: 8,
                marginTop: 12,
                paddingTop: 12,
                borderTopWidth: 1,
                borderColor,
            }}
        >
            <Text style={{ fontSize: 13, fontWeight: '600', color: foregroundColor }}>Members</Text>

            {members.map(m => {
                const isOwner = m.role === 'owner'
                const canRemove = !(isOwner && ownerCount <= 1)

                return (
                    <View
                        key={m.id}
                        style={{
                            flexDirection: 'row',
                            justifyContent: 'space-between',
                            alignItems: 'center',
                            paddingVertical: 4,
                        }}
                    >
                        <View style={{ flex: 1 }}>
                            <Text style={{ fontSize: 13, color: foregroundColor }}>
                                {m.userName}
                            </Text>
                            <Text style={{ fontSize: 11, color: mutedColor }}>{m.role}</Text>
                        </View>

                        <View style={{ flexDirection: 'row', gap: 8 }}>
                            <Pressable
                                onPress={() =>
                                    toggleRoleMutation.mutate({
                                        memberId: m.id,
                                        newRole: isOwner ? 'member' : 'owner',
                                    })
                                }
                                disabled={isOwner && ownerCount <= 1}
                                style={{
                                    padding: 4,
                                    opacity: isOwner && ownerCount <= 1 ? 0.4 : 1,
                                }}
                            >
                                <Text style={{ fontSize: 13 }}>
                                    {isOwner ? 'Make member' : 'Make owner'}
                                </Text>
                            </Pressable>

                            <Pressable
                                onPress={() => removeMemberMutation.mutate(m.id)}
                                disabled={!canRemove}
                                style={{ padding: 4, opacity: canRemove ? 1 : 0.4 }}
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
    const accentColor = useThemeColor('accent')
    const primaryColor = useThemeColor('primary')
    const primaryFgColor = useThemeColor('primary-foreground')
    const borderColor = useThemeColor('border')

    if (!isVisible) return null

    return (
        <View style={{ gap: 8 }}>
            <Text style={{ fontSize: 13, color: mutedColor }}>Select a member to add:</Text>
            <View style={{ flexDirection: 'row', gap: 4, flexWrap: 'wrap' }}>
                {availableMembers.map(uo => {
                    const isSelected = selectedUserOrg === uo.userOrgId
                    return (
                        <Pressable
                            key={uo.userOrgId}
                            onPress={() => onSelect(uo.userOrgId)}
                            style={{
                                borderWidth: 1,
                                borderColor: isSelected ? accentColor : borderColor,
                                borderRadius: 12,
                                paddingHorizontal: 12,
                                paddingVertical: 6,
                                backgroundColor: isSelected ? `${accentColor}14` : undefined,
                            }}
                        >
                            <Text style={{ fontSize: 13 }}>{uo.userName}</Text>
                        </Pressable>
                    )
                })}
            </View>
            <View style={{ flexDirection: 'row', gap: 8 }}>
                <Pressable
                    onPress={onAdd}
                    disabled={!selectedUserOrg || isPending}
                    style={{
                        backgroundColor: primaryColor,
                        paddingHorizontal: 16,
                        paddingVertical: 10,
                        borderRadius: 8,
                        opacity: !selectedUserOrg || isPending ? 0.5 : 1,
                    }}
                >
                    <Text style={{ color: primaryFgColor }}>Add</Text>
                </Pressable>
                <Pressable
                    onPress={onCancel}
                    style={{ paddingHorizontal: 16, paddingVertical: 10, borderRadius: 8 }}
                >
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
            style={{
                flexDirection: 'row',
                gap: 4,
                alignItems: 'center',
                padding: 4,
                opacity: disabled ? 0.4 : 1,
            }}
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
    const [mailboxesCollection, membersCollection] = useStore(
        'mail_mailboxes',
        'mail_mailbox_members'
    )

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

    const onSubmit = handleSubmit(data => createMutation.mutate(data))
    const canSubmit = !createMutation.isPending && isDirty

    return (
        <View style={{ gap: 12 }}>
            <Text style={{ fontSize: 18, fontWeight: 'bold', color: foregroundColor }}>
                Create Shared Mailbox
            </Text>

            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

            <TextInput control={control} name="address" label="Address" placeholder="support" />

            <SelectInput control={control} name="domain" label="Domain" options={domainOptions} />

            <TextInput
                control={control}
                name="display_name"
                label="Display Name"
                placeholder="Support Team"
            />

            <TextInput control={control} name="name" label="Mailbox Name" placeholder="Acme Corp" />

            <Pressable
                onPress={onSubmit}
                disabled={!canSubmit}
                style={{
                    flexDirection: 'row',
                    alignItems: 'center',
                    justifyContent: 'center',
                    gap: 8,
                    backgroundColor: primaryColor,
                    height: 44,
                    borderRadius: 8,
                    opacity: canSubmit ? 1 : 0.5,
                }}
            >
                <Plus size={16} color={primaryFgColor} />
                <Text style={{ fontWeight: '600', color: primaryFgColor }}>
                    {createMutation.isPending ? 'Creating...' : 'Create Mailbox'}
                </Text>
            </Pressable>
        </View>
    )
}
