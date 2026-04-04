import { ChevronLeft, ChevronRight, MoreVertical, RefreshCw, Square } from 'lucide-react-native'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'

interface EmailListToolbarProps {
    emailCount: number
}

export function EmailListToolbar({ emailCount }: EmailListToolbarProps) {
    const theme = useTheme()
    const breakpoint = useBreakpoint()

    if (breakpoint === 'mobile') return null

    const paginationText =
        emailCount > 0 ? `1\u2013${emailCount} of ${emailCount}` : 'No conversations'

    return (
        <View style={[styles.toolbar, { borderBottomColor: theme.borderColor.val }]}>
            <View style={styles.left}>
                <Pressable style={styles.checkbox}>
                    <Square size={18} color={theme.color8.val} />
                </Pressable>
                <Pressable style={styles.iconButton}>
                    <RefreshCw size={18} color={theme.color8.val} />
                </Pressable>
                <Pressable style={styles.iconButton}>
                    <MoreVertical size={18} color={theme.color8.val} />
                </Pressable>
            </View>
            <View style={styles.right}>
                <Text style={[styles.paginationText, { color: theme.color8.val }]}>
                    {paginationText}
                </Text>
                <Pressable style={styles.iconButton}>
                    <ChevronLeft size={18} color={theme.color8.val} />
                </Pressable>
                <Pressable style={styles.iconButton}>
                    <ChevronRight size={18} color={theme.color8.val} />
                </Pressable>
            </View>
        </View>
    )
}

const styles = StyleSheet.create({
    toolbar: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        height: 44,
        paddingHorizontal: 8,
        borderBottomWidth: 1,
    },
    left: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 2,
    },
    right: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 2,
    },
    checkbox: {
        padding: 8,
    },
    iconButton: {
        padding: 8,
        borderRadius: 20,
    },
    paginationText: {
        fontSize: 12,
        marginRight: 4,
    },
})
