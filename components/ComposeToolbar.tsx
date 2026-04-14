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
import { useCallback, useMemo } from 'react'
import { ActivityIndicator, Alert, Platform, Pressable, Text, View } from 'react-native'
import { ResponsiveToolbar, type ToolbarItem } from '~/components/ResponsiveToolbar'
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
    const activeColor = useThemeColor('primary')
    const primaryColor = useThemeColor('primary')
    const primaryFgColor = useThemeColor('primary-foreground')
    const borderColor = useThemeColor('border')
    const editorState = useBridgeState(editor)

    const handleLink = useCallback(() => {
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
    }, [editor, editorState.activeLink])

    const items: ToolbarItem[] = useMemo(
        () => [
            {
                type: 'custom',
                key: 'send',
                element: (
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
                            <Text
                                style={{ fontSize: 14, fontWeight: '600', color: primaryFgColor }}
                            >
                                Send
                            </Text>
                        )}
                    </Pressable>
                ),
            },
            {
                type: 'custom',
                key: 'bold',
                element: (
                    <FormatButton
                        icon={Bold}
                        isActive={editorState.isBoldActive}
                        onPress={() => editor.toggleBold()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflowLabel: 'Bold',
                overflowIcon: Bold,
                overflowPress: () => editor.toggleBold(),
            },
            {
                type: 'custom',
                key: 'italic',
                element: (
                    <FormatButton
                        icon={Italic}
                        isActive={editorState.isItalicActive}
                        onPress={() => editor.toggleItalic()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflowLabel: 'Italic',
                overflowIcon: Italic,
                overflowPress: () => editor.toggleItalic(),
            },
            {
                type: 'custom',
                key: 'underline',
                element: (
                    <FormatButton
                        icon={Underline}
                        isActive={editorState.isUnderlineActive}
                        onPress={() => editor.toggleUnderline()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflowLabel: 'Underline',
                overflowIcon: Underline,
                overflowPress: () => editor.toggleUnderline(),
            },
            { type: 'separator' },
            {
                type: 'custom',
                key: 'bullet-list',
                element: (
                    <FormatButton
                        icon={List}
                        isActive={editorState.isBulletListActive}
                        onPress={() => editor.toggleBulletList()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflowLabel: 'Bullet list',
                overflowIcon: List,
                overflowPress: () => editor.toggleBulletList(),
            },
            {
                type: 'custom',
                key: 'ordered-list',
                element: (
                    <FormatButton
                        icon={ListOrdered}
                        isActive={editorState.isOrderedListActive}
                        onPress={() => editor.toggleOrderedList()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflowLabel: 'Numbered list',
                overflowIcon: ListOrdered,
                overflowPress: () => editor.toggleOrderedList(),
            },
            { type: 'separator' },
            {
                type: 'custom',
                key: 'blockquote',
                element: (
                    <FormatButton
                        icon={Quote}
                        isActive={editorState.isBlockquoteActive}
                        onPress={() => editor.toggleBlockquote()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflowLabel: 'Blockquote',
                overflowIcon: Quote,
                overflowPress: () => editor.toggleBlockquote(),
            },
            {
                type: 'custom',
                key: 'link',
                element: (
                    <FormatButton
                        icon={Link2}
                        isActive={editorState.isLinkActive}
                        onPress={handleLink}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflowLabel: 'Link',
                overflowIcon: Link2,
                overflowPress: handleLink,
            },
            { type: 'separator' },
            {
                type: 'button',
                key: 'attach',
                icon: Paperclip,
                label: 'Attach',
                onPress: onAttach ?? (() => {}),
            },
        ],
        [
            primaryColor,
            primaryFgColor,
            isPending,
            onSend,
            editor,
            editorState,
            iconColor,
            activeColor,
            handleLink,
            onAttach,
        ]
    )

    const rightItems: ToolbarItem[] = useMemo(
        () => [
            { type: 'button', key: 'discard', icon: Trash2, label: 'Discard', onPress: onDiscard },
        ],
        [onDiscard]
    )

    return (
        <View style={{ borderTopWidth: 1, borderTopColor: borderColor }}>
            <ResponsiveToolbar items={items} rightItems={rightItems} />
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
