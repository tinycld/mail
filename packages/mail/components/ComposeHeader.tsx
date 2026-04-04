import { Maximize2, Minimize2, X } from 'lucide-react-native'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import type { ComposeMode } from '../hooks/useComposeState'

interface ComposeHeaderProps {
    mode: ComposeMode
    onMinimize: () => void
    onMaximize: () => void
    onClose: () => void
}

export function ComposeHeader({ mode, onMinimize, onMaximize, onClose }: ComposeHeaderProps) {
    const theme = useTheme()
    const breakpoint = useBreakpoint()
    const showWindowControls = breakpoint === 'desktop'

    return (
        <View style={[styles.header, { backgroundColor: theme.color.val }]}>
            <Pressable
                style={styles.titleArea}
                onPress={mode === 'minimized' ? onMinimize : undefined}
            >
                <Text style={[styles.title, { color: theme.background.val }]}>New Message</Text>
            </Pressable>
            <View style={styles.actions}>
                {showWindowControls ? (
                    <>
                        <Pressable style={styles.headerButton} onPress={onMinimize}>
                            <Minimize2 size={14} color={theme.background.val} />
                        </Pressable>
                        <Pressable style={styles.headerButton} onPress={onMaximize}>
                            <Maximize2 size={14} color={theme.background.val} />
                        </Pressable>
                    </>
                ) : null}
                <Pressable style={styles.headerButton} onPress={onClose}>
                    <X size={14} color={theme.background.val} />
                </Pressable>
            </View>
        </View>
    )
}

const styles = StyleSheet.create({
    header: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        height: 40,
        paddingHorizontal: 12,
        borderTopLeftRadius: 8,
        borderTopRightRadius: 8,
    },
    titleArea: {
        flex: 1,
    },
    title: {
        fontSize: 14,
        fontWeight: '600',
    },
    actions: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 4,
    },
    headerButton: {
        padding: 6,
        borderRadius: 4,
    },
})
