import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useServerActionState } from '@tinycld/core/lib/use-writes-available'
import { X } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'

export type Role = 'owner' | 'member'

interface Props {
    name: string
    email?: string
    isYou?: boolean
    role: Role
    canRemove: boolean
    canToggleRole: boolean
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
    onToggleRole,
    onRemove,
}: Props) {
    const mutedColor = useThemeColor('muted-foreground')
    const primaryColor = useThemeColor('primary')
    const dangerColor = useThemeColor('danger')
    const removeState = useServerActionState({
        isDisabled: !canRemove,
        accessibilityHint: `Removes ${name} from the mailbox`,
    })
    const isOwner = role === 'owner'
    const pillBg = isOwner ? `${primaryColor}1F` : `${mutedColor}26`
    const pillFg = isOwner ? primaryColor : mutedColor

    return (
        <View className="flex-row items-center gap-3 py-2">
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
            </View>
            <Pressable
                onPress={onToggleRole}
                disabled={!canToggleRole}
                className="rounded-md"
                style={{
                    paddingVertical: 3,
                    paddingHorizontal: 8,
                    backgroundColor: pillBg,
                    opacity: canToggleRole ? 1 : 0.5,
                }}
            >
                <Text style={{ fontSize: 11, fontWeight: '600', color: pillFg }}>
                    {isOwner ? 'Owner' : 'Member'}
                </Text>
            </Pressable>
            <Pressable
                onPress={onRemove}
                disabled={removeState.isDisabled}
                accessibilityRole="button"
                accessibilityLabel={`Remove ${name}`}
                accessibilityHint={removeState.accessibilityHint}
                className="p-1"
                style={{ opacity: removeState.isDisabled ? 0.35 : 1 }}
            >
                <X size={14} color={dangerColor} />
            </Pressable>
        </View>
    )
}
