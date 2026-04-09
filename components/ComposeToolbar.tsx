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
import { ActivityIndicator, Alert, Platform, Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'

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
    const theme = useTheme()
    const editorState = useBridgeState(editor)
    const iconColor = theme.color8.val
    const activeColor = theme.accentBackground.val

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
        <View style={[styles.toolbar, { borderTopColor: theme.borderColor.val }]}>
            <Pressable
                style={[
                    styles.sendButton,
                    { backgroundColor: activeColor },
                    isPending && styles.sendButtonDisabled,
                ]}
                onPress={onSend}
                disabled={isPending}
            >
                {isPending ? (
                    <ActivityIndicator size="small" color={theme.accentColor.val} />
                ) : (
                    <Text style={[styles.sendText, { color: theme.accentColor.val }]}>Send</Text>
                )}
            </Pressable>

            <View style={styles.formatGroup}>
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

            <View style={[styles.separator, { backgroundColor: theme.borderColor.val }]} />

            <View style={styles.formatGroup}>
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

            <View style={[styles.separator, { backgroundColor: theme.borderColor.val }]} />

            <View style={styles.formatGroup}>
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

            <View style={[styles.separator, { backgroundColor: theme.borderColor.val }]} />

            <Pressable style={styles.iconButton} onPress={onAttach}>
                <Paperclip size={16} color={iconColor} />
            </Pressable>

            <View style={styles.spacer} />

            <Pressable style={styles.iconButton} onPress={onDiscard}>
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
            style={[styles.iconButton, isActive && { backgroundColor: `${activeColor}22` }]}
            onPress={onPress}
        >
            <Icon size={16} color={isActive ? activeColor : iconColor} />
        </Pressable>
    )
}

const styles = StyleSheet.create({
    toolbar: {
        flexDirection: 'row',
        alignItems: 'center',
        height: 44,
        paddingHorizontal: 12,
        borderTopWidth: 1,
        gap: 2,
    },
    sendButton: {
        paddingHorizontal: 20,
        paddingVertical: 6,
        borderRadius: 20,
        minWidth: 72,
        alignItems: 'center',
    },
    sendButtonDisabled: {
        opacity: 0.6,
    },
    sendText: {
        fontSize: 14,
        fontWeight: '600',
    },
    formatGroup: {
        flexDirection: 'row',
        alignItems: 'center',
    },
    separator: {
        width: 1,
        height: 20,
        marginHorizontal: 4,
    },
    iconButton: {
        padding: 6,
        borderRadius: 6,
    },
    spacer: {
        flex: 1,
    },
})
