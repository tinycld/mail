import { MoreVertical, Reply, Star } from 'lucide-react-native'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { LabelBadge } from './LabelBadge'
import type { MockEmail, MockLabel } from './mockData'
import { mockLabels } from './mockData'

interface EmailHeaderProps {
    email: MockEmail
}

export function EmailHeader({ email }: EmailHeaderProps) {
    const theme = useTheme()
    const breakpoint = useBreakpoint()
    const isMobile = breakpoint === 'mobile'

    const emailLabels = email.labels
        .map(id => mockLabels.find(l => l.id === id))
        .filter((l): l is MockLabel => l != null)

    const initials = email.sender
        .split(' ')
        .map(n => n[0])
        .join('')
        .toUpperCase()
        .slice(0, 2)

    return (
        <View style={styles.container}>
            <View style={styles.subjectRow}>
                <Text
                    style={[
                        styles.subject,
                        isMobile && styles.subjectMobile,
                        { color: theme.color.val },
                    ]}
                >
                    {email.subject}
                </Text>
                <View style={styles.labelRow}>
                    {emailLabels.map(label => (
                        <LabelBadge key={label.id} name={label.name} color={label.color} />
                    ))}
                </View>
            </View>

            <View style={[styles.senderRow, { borderBottomColor: theme.borderColor.val }]}>
                <View style={[styles.avatar, { backgroundColor: theme.accentBackground.val }]}>
                    <Text style={[styles.avatarText, { color: theme.accentColor.val }]}>
                        {initials}
                    </Text>
                </View>
                <View style={styles.senderInfo}>
                    <View style={styles.senderNameRow}>
                        <Text style={[styles.senderName, { color: theme.color.val }]}>
                            {email.sender}
                        </Text>
                        {isMobile ? null : (
                            <Text style={[styles.senderEmail, { color: theme.color8.val }]}>
                                {'<'}
                                {email.senderEmail}
                                {'>'}
                            </Text>
                        )}
                    </View>
                    <Text style={[styles.toLine, { color: theme.color8.val }]}>to me</Text>
                </View>
                <Text style={[styles.date, { color: theme.color8.val }]}>{email.date}</Text>
                <Pressable style={styles.iconButton}>
                    <Star
                        size={18}
                        color={email.isStarred ? theme.yellow8.val : theme.color8.val}
                        fill={email.isStarred ? theme.yellow8.val : 'transparent'}
                    />
                </Pressable>
                <Pressable style={styles.iconButton}>
                    <Reply size={18} color={theme.color8.val} />
                </Pressable>
                <Pressable style={styles.iconButton}>
                    <MoreVertical size={18} color={theme.color8.val} />
                </Pressable>
            </View>
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        gap: 0,
    },
    subjectRow: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 8,
        paddingHorizontal: 16,
        paddingVertical: 12,
        flexWrap: 'wrap',
    },
    subject: {
        fontSize: 22,
        fontWeight: '400',
    },
    subjectMobile: {
        fontSize: 18,
    },
    labelRow: {
        flexDirection: 'row',
        gap: 4,
    },
    senderRow: {
        flexDirection: 'row',
        alignItems: 'center',
        paddingHorizontal: 16,
        paddingVertical: 12,
        gap: 8,
        borderBottomWidth: 1,
    },
    avatar: {
        width: 40,
        height: 40,
        borderRadius: 20,
        alignItems: 'center',
        justifyContent: 'center',
    },
    avatarText: {
        fontSize: 14,
        fontWeight: '600',
    },
    senderInfo: {
        flex: 1,
        gap: 2,
    },
    senderNameRow: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 4,
    },
    senderName: {
        fontSize: 14,
        fontWeight: '600',
    },
    senderEmail: {
        fontSize: 12,
    },
    toLine: {
        fontSize: 12,
    },
    date: {
        fontSize: 12,
        flexShrink: 0,
    },
    iconButton: {
        padding: 6,
        borderRadius: 20,
    },
})
