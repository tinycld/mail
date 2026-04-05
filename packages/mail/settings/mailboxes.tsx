import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { Lock, Mail, Plus, Trash2, UserPlus, X } from 'lucide-react-native'
import type { OneRouter } from 'one'
import { Link } from 'one'
import { newRecordId } from 'pbtsdb'
import { useState } from 'react'
import { Button, H4, ScrollView, Separator, SizableText, useTheme, XStack, YStack } from 'tamagui'
import { handleMutationErrorsWithForm } from '~/lib/errors'
import { useMutation } from '~/lib/mutations'
import { useStore } from '~/lib/pocketbase'
import { useCurrentRole } from '~/lib/use-current-role'
import { useOrgInfo } from '~/lib/use-org-info'
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
    const theme = useTheme()
    const { orgId } = useOrgInfo()
    const { isAdmin, userOrgId } = useCurrentRole()
    const [expandedMailbox, setExpandedMailbox] = useState<string | null>(null)
    const data = useMailboxData(orgId)

    if (!isAdmin) {
        return (
            <YStack flex={1} padding="$5" justifyContent="center" alignItems="center">
                <SizableText color="$color8">Admin access required</SizableText>
            </YStack>
        )
    }

    const domainOptions = data.domains.map(d => ({ label: d.domain, value: d.id }))
    const hasDomains = data.domains.length > 0

    return (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} backgroundColor="$background">
            <YStack flex={1} padding="$5" gap="$5" maxWidth={600}>
                <YStack gap="$2">
                    <Mail size={32} color={theme.colorFocus.val} />
                    <SizableText size="$6" fontWeight="bold" color="$color">
                        Mailboxes
                    </SizableText>
                    <SizableText size="$3" color="$color8">
                        Manage mailboxes and member assignments for your organization.
                    </SizableText>
                </YStack>

                <NoDomainsBanner isVisible={!hasDomains} />

                {hasDomains && (
                    <>
                        <YStack gap="$3">
                            <H4 color="$color">Mailboxes</H4>
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
                        </YStack>

                        <Separator />

                        <CreateMailboxForm domainOptions={domainOptions} userOrgId={userOrgId} />
                    </>
                )}
            </YStack>
        </ScrollView>
    )
}

function NoDomainsBanner({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <SizableText color="$color8">
            No mail domains configured.{' '}
            <Link href={'/app/settings/mail/provider' as OneRouter.Href}>
                <SizableText color="$blue10" textDecorationLine="underline">
                    Add a domain in Provider settings
                </SizableText>
            </Link>{' '}
            first.
        </SizableText>
    )
}

function NoMailboxesBanner({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return <SizableText color="$color8">No mailboxes yet.</SizableText>
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
    const isPersonal = mailbox.type === 'personal'
    const fullAddress = `${mailbox.address}@${mailbox.domainName}`

    return (
        <YStack borderWidth={1} borderColor="$borderColor" borderRadius="$3" padding="$3">
            <XStack
                justifyContent="space-between"
                alignItems="center"
                pressStyle={{ opacity: 0.7 }}
                onPress={onToggle}
                cursor="pointer"
            >
                <YStack gap="$1" flex={1}>
                    <XStack gap="$2" alignItems="center">
                        {isPersonal && <Lock size={14} color="$color8" />}
                        <SizableText fontWeight="600" color="$color">
                            {fullAddress}
                        </SizableText>
                        <TypeBadge type={mailbox.type} />
                    </XStack>
                    <SizableText size="$2" color="$color8">
                        {mailbox.name ? `${mailbox.name} · ` : ''}
                        {mailbox.displayName || 'No display name'} · {members.length} member
                        {members.length !== 1 ? 's' : ''}
                    </SizableText>
                </YStack>

                <DeleteMailboxButton mailboxId={mailbox.id} isVisible={!isPersonal} />
            </XStack>

            <MailboxMemberPanel
                isVisible={isExpanded}
                mailboxId={mailbox.id}
                members={members}
                orgMembers={orgMembers}
            />
        </YStack>
    )
}

function TypeBadge({ type }: { type: string }) {
    return (
        <XStack
            backgroundColor={type === 'personal' ? '$blue3' : '$green3'}
            paddingHorizontal="$2"
            paddingVertical="$0.5"
            borderRadius="$2"
        >
            <SizableText size="$1" color={type === 'personal' ? '$blue10' : '$green10'}>
                {type}
            </SizableText>
        </XStack>
    )
}

function DeleteMailboxButton({ mailboxId, isVisible }: { mailboxId: string; isVisible: boolean }) {
    const [mailboxesCollection] = useStore('mail_mailboxes')
    const [confirming, setConfirming] = useState(false)

    const deleteMutation = useMutation({
        mutationFn: function* () {
            yield mailboxesCollection.delete(mailboxId)
        },
        onSuccess: () => setConfirming(false),
    })

    if (!isVisible) return null

    if (confirming) {
        return (
            <XStack gap="$2">
                <Button size="$2" theme="red" onPress={() => deleteMutation.mutate()}>
                    <Button.Text>Confirm</Button.Text>
                </Button>
                <Button size="$2" onPress={() => setConfirming(false)}>
                    <Button.Text>Cancel</Button.Text>
                </Button>
            </XStack>
        )
    }

    return (
        <Button
            size="$2"
            chromeless
            onPress={e => {
                e.stopPropagation()
                setConfirming(true)
            }}
        >
            <Trash2 size={16} color="$red10" />
        </Button>
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
    const [membersCollection] = useStore('mail_mailbox_members')
    const [addingMember, setAddingMember] = useState(false)
    const [selectedUserOrg, setSelectedUserOrg] = useState('')

    const existingUserOrgIds = new Set(members.map(m => m.userOrgId))
    const availableMembers = orgMembers.filter(uo => !existingUserOrgIds.has(uo.userOrgId))

    const addMemberMutation = useMutation({
        mutationFn: function* () {
            yield membersCollection.insert({
                id: newRecordId(),
                mailbox: mailboxId,
                user_org: selectedUserOrg,
                role: 'member',
            })
        },
        onSuccess: () => {
            setAddingMember(false)
            setSelectedUserOrg('')
        },
    })

    const removeMemberMutation = useMutation({
        mutationFn: function* (memberId: string) {
            yield membersCollection.delete(memberId)
        },
    })

    const toggleRoleMutation = useMutation({
        mutationFn: function* ({ memberId, newRole }: { memberId: string; newRole: string }) {
            yield membersCollection.update(memberId, draft => {
                draft.role = newRole as 'owner' | 'member'
            })
        },
    })

    if (!isVisible) return null

    const ownerCount = members.filter(m => m.role === 'owner').length

    return (
        <YStack
            gap="$2"
            marginTop="$3"
            paddingTop="$3"
            borderTopWidth={1}
            borderColor="$borderColor"
        >
            <SizableText size="$3" fontWeight="600" color="$color">
                Members
            </SizableText>

            {members.map(m => {
                const isOwner = m.role === 'owner'
                const canRemove = !(isOwner && ownerCount <= 1)

                return (
                    <XStack
                        key={m.id}
                        justifyContent="space-between"
                        alignItems="center"
                        paddingVertical="$1"
                    >
                        <YStack flex={1}>
                            <SizableText size="$3" color="$color">
                                {m.userName}
                            </SizableText>
                            <SizableText size="$1" color="$color8">
                                {m.role}
                            </SizableText>
                        </YStack>

                        <XStack gap="$2">
                            <Button
                                size="$2"
                                chromeless
                                onPress={() =>
                                    toggleRoleMutation.mutate({
                                        memberId: m.id,
                                        newRole: isOwner ? 'member' : 'owner',
                                    })
                                }
                                disabled={isOwner && ownerCount <= 1}
                                opacity={isOwner && ownerCount <= 1 ? 0.4 : 1}
                            >
                                <Button.Text size="$2">
                                    {isOwner ? 'Make member' : 'Make owner'}
                                </Button.Text>
                            </Button>

                            <Button
                                size="$2"
                                chromeless
                                onPress={() => removeMemberMutation.mutate(m.id)}
                                disabled={!canRemove}
                                opacity={canRemove ? 1 : 0.4}
                            >
                                <X size={14} color="$red10" />
                            </Button>
                        </XStack>
                    </XStack>
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
        </YStack>
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
    if (!isVisible) return null

    return (
        <YStack gap="$2">
            <SizableText size="$2" color="$color8">
                Select a member to add:
            </SizableText>
            <XStack gap="$1" flexWrap="wrap">
                {availableMembers.map(uo => {
                    const isSelected = selectedUserOrg === uo.userOrgId
                    return (
                        <Button
                            key={uo.userOrgId}
                            onPress={() => onSelect(uo.userOrgId)}
                            theme={isSelected ? 'accent' : undefined}
                            borderColor={isSelected ? '$accentBackground' : '$borderColor'}
                            borderWidth={1}
                            borderRadius="$3"
                            size="$2"
                        >
                            <Button.Text size="$2">{uo.userName}</Button.Text>
                        </Button>
                    )
                })}
            </XStack>
            <XStack gap="$2">
                <Button
                    size="$3"
                    theme="accent"
                    onPress={onAdd}
                    disabled={!selectedUserOrg || isPending}
                    opacity={!selectedUserOrg || isPending ? 0.5 : 1}
                >
                    <Button.Text>Add</Button.Text>
                </Button>
                <Button size="$3" onPress={onCancel}>
                    <Button.Text>Cancel</Button.Text>
                </Button>
            </XStack>
        </YStack>
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
        <Button
            size="$2"
            chromeless
            onPress={onPress}
            disabled={disabled}
            opacity={disabled ? 0.4 : 1}
        >
            <XStack gap="$1" alignItems="center">
                <UserPlus size={14} />
                <SizableText size="$2">Add member</SizableText>
            </XStack>
        </Button>
    )
}

function CreateMailboxForm({
    domainOptions,
    userOrgId,
}: {
    domainOptions: { label: string; value: string }[]
    userOrgId: string
}) {
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
        mutationFn: function* (data: z.infer<typeof createMailboxSchema>) {
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
        },
        onSuccess: () => reset(),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const onSubmit = handleSubmit(data => createMutation.mutate(data))
    const canSubmit = !createMutation.isPending && isDirty

    return (
        <YStack gap="$3">
            <H4 color="$color">Create Shared Mailbox</H4>

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

            <Button
                theme="accent"
                size="$4"
                onPress={onSubmit}
                disabled={!canSubmit}
                opacity={canSubmit ? 1 : 0.5}
            >
                <XStack gap="$2" alignItems="center">
                    <Plus size={16} />
                    <Button.Text fontWeight="600">
                        {createMutation.isPending ? 'Creating...' : 'Create Mailbox'}
                    </Button.Text>
                </XStack>
            </Button>
        </YStack>
    )
}
