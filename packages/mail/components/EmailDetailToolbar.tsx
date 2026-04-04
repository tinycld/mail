import {
    Archive,
    ArrowLeft,
    ChevronLeft,
    ChevronRight,
    CircleAlert,
    FolderInput,
    MailOpen,
    MoreVertical,
    Tag,
    Trash2,
} from 'lucide-react-native'
import { useRouter } from 'one'
import { Pressable, StyleSheet, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'

export function EmailDetailToolbar() {
    const theme = useTheme()
    const router = useRouter()
    const breakpoint = useBreakpoint()
    const isMobile = breakpoint === 'mobile'

    return (
        <View style={[styles.toolbar, { borderBottomColor: theme.borderColor.val }]}>
            <View style={styles.left}>
                <Pressable style={styles.iconButton} onPress={() => router.back()}>
                    <ArrowLeft size={18} color={theme.color8.val} />
                </Pressable>
                <Pressable style={styles.iconButton}>
                    <Archive size={18} color={theme.color8.val} />
                </Pressable>
                {isMobile ? null : (
                    <Pressable style={styles.iconButton}>
                        <CircleAlert size={18} color={theme.color8.val} />
                    </Pressable>
                )}
                <Pressable style={styles.iconButton}>
                    <Trash2 size={18} color={theme.color8.val} />
                </Pressable>
                {isMobile ? null : (
                    <>
                        <View
                            style={[styles.separator, { backgroundColor: theme.borderColor.val }]}
                        />
                        <Pressable style={styles.iconButton}>
                            <FolderInput size={18} color={theme.color8.val} />
                        </Pressable>
                        <Pressable style={styles.iconButton}>
                            <Tag size={18} color={theme.color8.val} />
                        </Pressable>
                    </>
                )}
                <Pressable style={styles.iconButton}>
                    <MailOpen size={18} color={theme.color8.val} />
                </Pressable>
                <Pressable style={styles.iconButton}>
                    <MoreVertical size={18} color={theme.color8.val} />
                </Pressable>
            </View>
            {isMobile ? null : (
                <View style={styles.right}>
                    <Pressable style={styles.iconButton}>
                        <ChevronLeft size={18} color={theme.color8.val} />
                    </Pressable>
                    <Pressable style={styles.iconButton}>
                        <ChevronRight size={18} color={theme.color8.val} />
                    </Pressable>
                </View>
            )}
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
    iconButton: {
        padding: 8,
        borderRadius: 20,
    },
    separator: {
        width: 1,
        height: 20,
        marginHorizontal: 4,
    },
})
