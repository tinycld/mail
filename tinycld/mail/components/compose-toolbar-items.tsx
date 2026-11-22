import type { ToolbarItem } from '@tinycld/core/components/ResponsiveToolbar'
import { ButtonSpinner, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { Paperclip, Trash2 } from 'lucide-react-native'

interface ComposeToolbarItemsInput {
    onSend: () => void
    onAttach: () => void
    onDiscard: () => void
    isPending: boolean
    isSendDisabled: boolean
}

/**
 * Mail's own items around the core `RichTextToolbar` formats: Send first,
 * Attach after the formats, Discard at the right edge. ComposeWindow and
 * InlineComposeForm both draw them.
 */
export function composeToolbarItems({
    onSend,
    onAttach,
    onDiscard,
    isPending,
    isSendDisabled,
}: ComposeToolbarItemsInput): {
    leadingItems: ToolbarItem[]
    trailingItems: ToolbarItem[]
    rightItems: ToolbarItem[]
} {
    const sendLabel = isPending ? (
        <ButtonSpinner />
    ) : (
        <ButtonText style={{ fontSize: 14, fontWeight: '600' }}>Send</ButtonText>
    )
    return {
        leadingItems: [
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
                        {sendLabel}
                    </ServerActionButton>
                ),
            },
        ],
        trailingItems: [
            { type: 'button', key: 'attach', icon: Paperclip, label: 'Attach', onPress: onAttach },
        ],
        rightItems: [
            { type: 'button', key: 'discard', icon: Trash2, label: 'Discard', onPress: onDiscard },
        ],
    }
}
