import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useServerActionState } from '@tinycld/core/lib/use-writes-available'
import { X } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'
import { viaGroupLabel } from './member-rows'

export type Role = 'owner' | 'member'

interface Props {
    name: string
    email?: string
    isYou?: boolean
    role: Role
    canRemove: boolean
    canToggleRole: boolean
    /** False for a viewer who cannot change members: the remove control is not shown. */
    canEdit?: boolean
    /** True when a group gives this access: it can only change by changing the group grant. */
    isDerived?: boolean
    /** Names of the groups that give this access, for the note under the name. */
    groupNames?: string[]
    onToggleRole: () => void
    onRemove: () => void
}

export function MailboxMemberRow({
    name,
    email,
    isYou = false,
    role,
    canRemove,
    canToggleRole,
    canEdit = true,
    isDerived = false,
    groupNames = [],
    onToggleRole,
    onRemove,
}: Props) {
    const mutedColor = useThemeColor('muted-foreground')
    const primaryColor = useThemeColor('primary')
    const dangerColor = useThemeColor('danger')
    const removeState = useServerActionState({
        isDisabled: !canRemove,
        accessibilityHint: `Removes ${name} from the inbox`,
    })
    const isOwner = role === 'owner'
    const pillBg = isOwner ? `${primaryColor}1F` : `${mutedColor}26`
    const pillFg = isOwner ? primaryColor : mutedColor

    return (
        <View
            testID={`mailbox-member-row-${email || name}`}
            className="flex-row items-center gap-3 py-2"
        >
            <View className="flex-1" style={{ minWidth: 0 }}>
                <Text className="text-foreground" style={{ fontSize: 13, fontWeight: '600' }}>
                    {name}
                    {isYou && (
                        <Text className="text-muted-foreground" style={{ fontWeight: '500' }}>
                            {' '}
                            · you
                        </Text>
                    )}
                </Text>
                {email ? (
                    <Text
                        className="text-muted-foreground"
                        style={{ fontSize: 11.5, marginTop: 1 }}
                    >
                        {email}
                    </Text>
                ) : null}
                <ViaGroupNote isVisible={isDerived} groupNames={groupNames} />
            </View>
            <Pressable
                onPress={onToggleRole}
                disabled={!canToggleRole || isDerived}
                className="rounded-md"
                style={{
                    paddingVertical: 3,
                    paddingHorizontal: 8,
                    backgroundColor: pillBg,
                    opacity: canToggleRole && !isDerived ? 1 : 0.5,
                }}
            >
                <Text style={{ fontSize: 11, fontWeight: '600', color: pillFg }}>
                    {isOwner ? 'Owner' : 'Member'}
                </Text>
            </Pressable>
            <RemoveButton
                isVisible={canEdit && !isDerived}
                name={name}
                state={removeState}
                color={dangerColor}
                onRemove={onRemove}
            />
        </View>
    )
}

function ViaGroupNote({ isVisible, groupNames }: { isVisible: boolean; groupNames: string[] }) {
    if (!isVisible) return null
    return (
        <Text className="text-muted-foreground" style={{ fontSize: 11.5, marginTop: 1 }}>
            {viaGroupLabel(groupNames)}
        </Text>
    )
}

function RemoveButton({
    isVisible,
    name,
    state,
    color,
    onRemove,
}: {
    isVisible: boolean
    name: string
    state: ReturnType<typeof useServerActionState>
    color: string
    onRemove: () => void
}) {
    if (!isVisible) return null
    return (
        <Pressable
            onPress={onRemove}
            disabled={state.isDisabled}
            accessibilityRole="button"
            accessibilityLabel={`Remove ${name}`}
            accessibilityHint={state.accessibilityHint}
            className="p-1"
            style={{ opacity: state.isDisabled ? 0.35 : 1 }}
        >
            <X size={14} color={color} />
        </Pressable>
    )
}
