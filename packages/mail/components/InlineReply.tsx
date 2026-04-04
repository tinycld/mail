import { Forward, Reply, ReplyAll } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, StyleSheet, Text, TextInput, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'

export function InlineReply() {
    const theme = useTheme()
    const [isExpanded, setIsExpanded] = useState(false)
    const breakpoint = useBreakpoint()
    const isMobile = breakpoint === 'mobile'

    if (isExpanded) {
        return (
            <View
                style={[
                    styles.replyBox,
                    {
                        borderColor: theme.borderColor.val,
                        backgroundColor: theme.background.val,
                    },
                ]}
            >
                <TextInput
                    style={[styles.replyInput, { color: theme.color.val }]}
                    placeholder="Click here to Reply"
                    placeholderTextColor={theme.placeholderColor.val}
                    multiline
                />
                <View style={styles.replyActions}>
                    <Pressable
                        style={[styles.sendButton, { backgroundColor: theme.accentBackground.val }]}
                    >
                        <Text style={[styles.sendText, { color: theme.accentColor.val }]}>
                            Send
                        </Text>
                    </Pressable>
                </View>
            </View>
        )
    }

    return (
        <View
            style={[
                styles.container,
                isMobile && styles.containerMobile,
                { borderTopColor: theme.borderColor.val },
            ]}
        >
            <Pressable
                style={[styles.actionButton, { borderColor: theme.borderColor.val }]}
                onPress={() => setIsExpanded(true)}
            >
                <Reply size={16} color={theme.color8.val} />
                <Text style={[styles.actionText, { color: theme.color8.val }]}>Reply</Text>
            </Pressable>
            <Pressable style={[styles.actionButton, { borderColor: theme.borderColor.val }]}>
                <ReplyAll size={16} color={theme.color8.val} />
                <Text style={[styles.actionText, { color: theme.color8.val }]}>Reply all</Text>
            </Pressable>
            <Pressable style={[styles.actionButton, { borderColor: theme.borderColor.val }]}>
                <Forward size={16} color={theme.color8.val} />
                <Text style={[styles.actionText, { color: theme.color8.val }]}>Forward</Text>
            </Pressable>
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        flexDirection: 'row',
        gap: 8,
        padding: 16,
        borderTopWidth: 1,
    },
    containerMobile: {
        flexWrap: 'wrap',
    },
    actionButton: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 6,
        paddingHorizontal: 16,
        paddingVertical: 8,
        borderRadius: 20,
        borderWidth: 1,
    },
    actionText: {
        fontSize: 13,
        fontWeight: '500',
    },
    replyBox: {
        margin: 16,
        borderWidth: 1,
        borderRadius: 8,
        padding: 12,
        minHeight: 120,
    },
    replyInput: {
        flex: 1,
        fontSize: 14,
        minHeight: 80,
        textAlignVertical: 'top',
    },
    replyActions: {
        flexDirection: 'row',
        justifyContent: 'flex-end',
        marginTop: 8,
    },
    sendButton: {
        paddingHorizontal: 20,
        paddingVertical: 8,
        borderRadius: 20,
    },
    sendText: {
        fontSize: 14,
        fontWeight: '600',
    },
})
