import { ResponsiveToolbar, type ToolbarItem } from '@tinycld/core/components/ResponsiveToolbar'
import type { EditorCommands, EditorToolbarState } from '@tinycld/core/lib/editor/types'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { ButtonSpinner, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
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
import { Alert, Platform, Pressable, View } from 'react-native'

interface ComposeToolbarProps {
    commands: EditorCommands
    toolbarState: EditorToolbarState
    onDiscard: () => void
    onSend: () => void
    onAttach?: () => void
    isPending: boolean
    isSendDisabled?: boolean
}

export function ComposeToolbar({
    commands,
    toolbarState,
    onDiscard,
    onSend,
    onAttach,
    isPending,
    isSendDisabled = false,
}: ComposeToolbarProps) {
    const iconColor = useThemeColor('muted-foreground')
    const activeColor = useThemeColor('primary')

    const handleLink = useCallback(() => {
        const defaultUrl = toolbarState.currentLink ?? 'https://'

        if (Platform.OS === 'web') {
            const url = window.prompt('Enter URL:', defaultUrl)
            if (url !== null) {
                if (url) {
                    commands.setLink(url)
                } else {
                    commands.removeLink()
                }
            }
        } else {
            Alert.prompt(
                'Insert Link',
                'Enter URL:',
                url => {
                    if (url !== null) {
                        if (url) {
                            commands.setLink(url)
                        } else {
                            commands.removeLink()
                        }
                    }
                },
                'plain-text',
                defaultUrl
            )
        }
    }, [commands, toolbarState.currentLink])

    const items: ToolbarItem[] = useMemo(
        () => [
            {
                type: 'custom',
                key: 'send',
                element: (
                    <ServerActionButton
                        className="rounded-full"
                        style={{ paddingHorizontal: 20, paddingVertical: 6, minWidth: 72 }}
                        onPress={onSend}
                        isDisabled={isPending || isSendDisabled}
                        accessibilityHint="Sends this email"
                    >
                        {isPending ? (
                            <ButtonSpinner />
                        ) : (
                            <ButtonText style={{ fontSize: 14, fontWeight: '600' }}>
                                Send
                            </ButtonText>
                        )}
                    </ServerActionButton>
                ),
            },
            {
                type: 'custom',
                key: 'bold',
                element: (
                    <FormatButton
                        icon={Bold}
                        label="Bold"
                        isActive={toolbarState.isBoldActive}
                        onPress={() => commands.toggleBold()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflow: { label: 'Bold', icon: Bold, onPress: () => commands.toggleBold() },
            },
            {
                type: 'custom',
                key: 'italic',
                element: (
                    <FormatButton
                        icon={Italic}
                        label="Italic"
                        isActive={toolbarState.isItalicActive}
                        onPress={() => commands.toggleItalic()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflow: { label: 'Italic', icon: Italic, onPress: () => commands.toggleItalic() },
            },
            {
                type: 'custom',
                key: 'underline',
                element: (
                    <FormatButton
                        icon={Underline}
                        label="Underline"
                        isActive={toolbarState.isUnderlineActive}
                        onPress={() => commands.toggleUnderline()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflow: {
                    label: 'Underline',
                    icon: Underline,
                    onPress: () => commands.toggleUnderline(),
                },
            },
            { type: 'separator' },
            {
                type: 'custom',
                key: 'bullet-list',
                element: (
                    <FormatButton
                        icon={List}
                        label="Bullet list"
                        isActive={toolbarState.isBulletListActive}
                        onPress={() => commands.toggleBulletList()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflow: {
                    label: 'Bullet list',
                    icon: List,
                    onPress: () => commands.toggleBulletList(),
                },
            },
            {
                type: 'custom',
                key: 'ordered-list',
                element: (
                    <FormatButton
                        icon={ListOrdered}
                        label="Numbered list"
                        isActive={toolbarState.isOrderedListActive}
                        onPress={() => commands.toggleOrderedList()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflow: {
                    label: 'Numbered list',
                    icon: ListOrdered,
                    onPress: () => commands.toggleOrderedList(),
                },
            },
            { type: 'separator' },
            {
                type: 'custom',
                key: 'blockquote',
                element: (
                    <FormatButton
                        icon={Quote}
                        label="Blockquote"
                        isActive={toolbarState.isBlockquoteActive}
                        onPress={() => commands.toggleBlockquote()}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflow: {
                    label: 'Blockquote',
                    icon: Quote,
                    onPress: () => commands.toggleBlockquote(),
                },
            },
            {
                type: 'custom',
                key: 'link',
                element: (
                    <FormatButton
                        icon={Link2}
                        label="Link"
                        isActive={toolbarState.isLinkActive}
                        onPress={handleLink}
                        iconColor={iconColor}
                        activeColor={activeColor}
                    />
                ),
                overflow: { label: 'Link', icon: Link2, onPress: handleLink },
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
            isPending,
            isSendDisabled,
            onSend,
            commands,
            toolbarState,
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
        <View className="border-t border-border">
            <ResponsiveToolbar items={items} rightItems={rightItems} />
        </View>
    )
}

// On web a mousedown blurs the editor and collapses its selection before the
// press is handled, so a format would apply to nothing. Web-only: native has
// no DOM focus, and React Native rejects the unknown prop.
const FOCUS_GUARD =
    Platform.OS === 'web'
        ? { onMouseDown: (e: { preventDefault: () => void }) => e.preventDefault() }
        : {}

interface FormatButtonProps {
    icon: React.ComponentType<{ size: number; color: string }>
    label: string
    isActive: boolean
    onPress: () => void
    iconColor: string
    activeColor: string
}

function FormatButton({
    icon: Icon,
    label,
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
            accessibilityRole="button"
            accessibilityLabel={label}
            accessibilityState={{ selected: isActive }}
            {...FOCUS_GUARD}
        >
            <Icon size={16} color={isActive ? activeColor : iconColor} />
        </Pressable>
    )
}
