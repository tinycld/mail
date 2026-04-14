import { type EditorBridge, useBridgeState } from '@10play/tentap-editor'
import {
    Bold,
    Italic,
    Link2,
    List,
    ListOrdered,
    Paperclip,
    Quote,
    Trash2,
    Underline,
} from 'lucide-react-native'
import { ActivityIndicator, Alert, Platform, Pressable, Text, View } from 'react-native'
import { useThemeColor } from '~/lib/use-app-theme'

interface ComposeToolbarProps {
    editor: EditorBridge
    onDiscard: () => void
    onSend: () => void
    onAttach?: () => void
    isPending: boolean
}

export function ComposeToolbar({
    editor,
    onDiscard,
    onSend,
    onAttach,
    isPending,
}: ComposeToolbarProps) {
    const iconColor = useThemeColor('muted-foreground')
    const activeColor = useThemeColor('accent')
    const primaryColor = useThemeColor('primary')
    const primaryFgColor = useThemeColor('primary-foreground')
    const borderColor = useThemeColor('border')
    const editorState = useBridgeState(editor)

    const handleLink = () => {
        const defaultUrl = editorState.activeLink ?? 'https://'

        if (Platform.OS === 'web') {
            const url = window.prompt('Enter URL:', defaultUrl)
            if (url !== null) {
                editor.setLink(url || '')
            }
        } else {
            Alert.prompt(
                'Insert Link',
                'Enter URL:',
                url => {
                    if (url !== null) {
                        editor.setLink(url || '')
                    }
                },
                'plain-text',
                defaultUrl
            )
        }
    }

    return (
        <View
            className="flex-row items-center px-3"
            style={{
                height: 44,
                borderTopWidth: 1,
                borderTopColor: borderColor,
                gap: 2,
            }}
        >
            <Pressable
                className="rounded-full items-center"
                style={[
                    {
                        paddingHorizontal: 20,
                        paddingVertical: 6,
                        minWidth: 72,
                        backgroundColor: primaryColor,
                    },
                    isPending && { opacity: 0.6 },
                ]}
                onPress={onSend}
                disabled={isPending}
            >
                {isPending ? (
                    <ActivityIndicator size="small" color={primaryFgColor} />
                ) : (
                    <Text style={{ fontSize: 14, fontWeight: '600', color: primaryFgColor }}>
                        Send
                    </Text>
                )}
            </Pressable>

            <View className="flex-row items-center">
                <FormatButton
                    icon={Bold}
                    isActive={editorState.isBoldActive}
                    onPress={() => editor.toggleBold()}
                    iconColor={iconColor}
                    activeColor={activeColor}
                />
                <FormatButton
                    icon={Italic}
                    isActive={editorState.isItalicActive}
                    onPress={() => editor.toggleItalic()}
                    iconColor={iconColor}
                    activeColor={activeColor}
                />
                <FormatButton
                    icon={Underline}
                    isActive={editorState.isUnderlineActive}
                    onPress={() => editor.toggleUnderline()}
                    iconColor={iconColor}
                    activeColor={activeColor}
                />
            </View>

            <View className="mx-1" style={{ width: 1, height: 20, backgroundColor: borderColor }} />

            <View className="flex-row items-center">
                <FormatButton
                    icon={List}
                    isActive={editorState.isBulletListActive}
                    onPress={() => editor.toggleBulletList()}
                    iconColor={iconColor}
                    activeColor={activeColor}
                />
                <FormatButton
                    icon={ListOrdered}
                    isActive={editorState.isOrderedListActive}
                    onPress={() => editor.toggleOrderedList()}
                    iconColor={iconColor}
                    activeColor={activeColor}
                />
            </View>

            <View className="mx-1" style={{ width: 1, height: 20, backgroundColor: borderColor }} />

            <View className="flex-row items-center">
                <FormatButton
                    icon={Quote}
                    isActive={editorState.isBlockquoteActive}
                    onPress={() => editor.toggleBlockquote()}
                    iconColor={iconColor}
                    activeColor={activeColor}
                />
                <FormatButton
                    icon={Link2}
                    isActive={editorState.isLinkActive}
                    onPress={handleLink}
                    iconColor={iconColor}
                    activeColor={activeColor}
                />
            </View>

            <View className="mx-1" style={{ width: 1, height: 20, backgroundColor: borderColor }} />

            <Pressable className="rounded-md" style={{ padding: 6 }} onPress={onAttach}>
                <Paperclip size={16} color={iconColor} />
            </Pressable>

            <View className="flex-1" />

            <Pressable className="rounded-md" style={{ padding: 6 }} onPress={onDiscard}>
                <Trash2 size={16} color={iconColor} />
            </Pressable>
        </View>
    )
}

interface FormatButtonProps {
    icon: React.ComponentType<{ size: number; color: string }>
    isActive: boolean
    onPress: () => void
    iconColor: string
    activeColor: string
}

function FormatButton({
    icon: Icon,
    isActive,
    onPress,
    iconColor,
    activeColor,
}: FormatButtonProps) {
    return (
        <Pressable
            className="rounded-md p-1.5"
            style={isActive ? { backgroundColor: `${activeColor}22` } : undefined}
            onPress={onPress}
        >
            <Icon size={16} color={isActive ? activeColor : iconColor} />
        </Pressable>
    )
}
