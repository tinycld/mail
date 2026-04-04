import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { ArrowLeft } from 'lucide-react-native'
import { useRouter } from 'one'
import { Pressable, StyleSheet, View } from 'react-native'
import { Button, ScrollView, SizableText, useTheme, XStack, YStack } from 'tamagui'
import { useMutation } from '~/lib/mutations'
import { useStore } from '~/lib/pocketbase'
import { useCurrentRole } from '~/lib/use-current-role'
import { useOrgInfo } from '~/lib/use-org-info'
import { SelectInput, TextInput, useForm, z, zodResolver } from '~/ui/form'

const ROLE_OPTIONS = [
    { label: 'Admin', value: 'admin' },
    { label: 'Member', value: 'member' },
]

const inviteSchema = z.object({
    email: z.string().email('Valid email is required'),
    role: z.enum(['admin', 'member']),
})

interface MemberRow {
    userOrgId: string
    userId: string
    name: string
    email: string
    role: 'admin' | 'member'
}

export default function MembersSettings() {
    const router = useRouter()
    const theme = useTheme()
    const { isAdmin } = useCurrentRole()
    const { orgId } = useOrgInfo()
    const [userOrgCollection] = useStore('user_org')

    const { data: userOrgs } = useLiveQuery(
        query =>
            query
                .from({ user_org: userOrgCollection })
                .where(({ user_org }) => eq(user_org.org, orgId)),
        [orgId]
    )

    const members: MemberRow[] = (userOrgs ?? []).map(uo => ({
        userOrgId: uo.id,
        userId: uo.user,
        name: uo.expand?.user?.name ?? '',
        email: uo.expand?.user?.email ?? '',
        role: uo.role,
    }))

    if (!isAdmin) {
        return (
            <YStack flex={1} padding="$5" alignItems="center" justifyContent="center" backgroundColor="$background">
                <SizableText size="$5" color="$color8">
                    Only admins can manage members.
                </SizableText>
            </YStack>
        )
    }

    return (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} backgroundColor="$background">
            <YStack flex={1} padding="$5" maxWidth={600}>
                <XStack gap="$3" alignItems="center" marginBottom="$5">
                    <Pressable onPress={() => router.back()}>
                        <ArrowLeft size={24} color={theme.color.val} />
                    </Pressable>
                    <SizableText size="$7" fontWeight="bold" color="$color">
                        Members
                    </SizableText>
                </XStack>

                <MemberList members={members} />
                <InviteForm />
            </YStack>
        </ScrollView>
    )
}

function MemberList({ members }: { members: MemberRow[] }) {
    const theme = useTheme()
    const [userOrgCollection] = useStore('user_org')

    const updateRole = useMutation({
        mutationFn: function* ({ userOrgId, role }: { userOrgId: string; role: string }) {
            yield userOrgCollection.update(userOrgId, draft => {
                draft.role = role as 'admin' | 'member'
            })
        },
    })

    const removeMember = useMutation({
        mutationFn: function* ({ userOrgId }: { userOrgId: string }) {
            yield userOrgCollection.delete(userOrgId)
        },
    })

    if (members.length === 0) {
        return (
            <SizableText size="$4" color="$color8" marginBottom="$5">
                No members found.
            </SizableText>
        )
    }

    return (
        <YStack marginBottom="$5" gap="$0">
            <SizableText
                size="$3"
                fontWeight="600"
                color="$colorFocus"
                textTransform="uppercase"
                letterSpacing={0.5}
                marginBottom="$2"
            >
                Current Members
            </SizableText>
            <View
                style={[
                    styles.listContainer,
                    {
                        backgroundColor: theme.backgroundHover.val,
                        borderColor: theme.borderColor.val,
                    },
                ]}
            >
                {members.map(member => (
                    <MemberRowItem
                        key={member.userOrgId}
                        member={member}
                        onToggleRole={updateRole.mutate}
                        onRemove={removeMember.mutate}
                    />
                ))}
            </View>
        </YStack>
    )
}

function MemberRowItem({
    member,
    onToggleRole,
    onRemove,
}: {
    member: MemberRow
    onToggleRole: (args: { userOrgId: string; role: string }) => void
    onRemove: (args: { userOrgId: string }) => void
}) {
    const theme = useTheme()
    const nextRole = member.role === 'admin' ? 'member' : 'admin'
    const badgeColor =
        member.role === 'admin' ? theme.accentBackground.val : theme.backgroundFocus.val

    const handleToggleRole = () => onToggleRole({ userOrgId: member.userOrgId, role: nextRole })
    const handleRemove = () => onRemove({ userOrgId: member.userOrgId })

    return (
        <View style={styles.memberRow}>
            <YStack flex={1} gap="$1">
                <SizableText size="$4" fontWeight="600" color="$color">
                    {member.name || member.email}
                </SizableText>
                <SizableText size="$3" color="$color8">
                    {member.email}
                </SizableText>
            </YStack>
            <XStack gap="$2" alignItems="center">
                <Pressable
                    onPress={handleToggleRole}
                    style={[styles.badge, { backgroundColor: badgeColor }]}
                >
                    <SizableText size="$2" fontWeight="600" color="$color">
                        {member.role}
                    </SizableText>
                </Pressable>
                <Button size="$2" theme="red" onPress={handleRemove}>
                    <Button.Text>Remove</Button.Text>
                </Button>
            </XStack>
        </View>
    )
}

function InviteForm() {
    const theme = useTheme()

    const { control, handleSubmit, reset } = useForm({
        mode: 'onChange',
        resolver: zodResolver(inviteSchema),
        defaultValues: { email: '', role: 'member' as const },
    })

    const onSubmit = handleSubmit(_data => {
        // TODO: call invite API endpoint
        reset()
    })

    return (
        <YStack gap="$3">
            <SizableText
                size="$3"
                fontWeight="600"
                color="$colorFocus"
                textTransform="uppercase"
                letterSpacing={0.5}
            >
                Invite Member
            </SizableText>
            <View
                style={[
                    styles.listContainer,
                    {
                        backgroundColor: theme.backgroundHover.val,
                        borderColor: theme.borderColor.val,
                    },
                ]}
            >
                <YStack padding="$3" gap="$3">
                    <TextInput control={control} name="email" label="Email" />
                    <SelectInput
                        control={control}
                        name="role"
                        label="Role"
                        options={ROLE_OPTIONS}
                    />
                    <Button theme="accent" size="$3" onPress={onSubmit}>
                        <Button.Text fontWeight="600">Send Invite</Button.Text>
                    </Button>
                </YStack>
            </View>
        </YStack>
    )
}

const styles = StyleSheet.create({
    listContainer: {
        borderRadius: 12,
        borderWidth: 1,
        overflow: 'hidden',
    },
    memberRow: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        paddingHorizontal: 16,
        paddingVertical: 12,
    },
    badge: {
        paddingHorizontal: 8,
        paddingVertical: 4,
        borderRadius: 6,
    },
})
