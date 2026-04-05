import { ChevronDown, ChevronUp, MoreVertical, Reply, Star } from 'lucide-react-native'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { LabelBadge } from './LabelBadge'
import { formatMailDate } from './thread-list-item'

interface ThreadSubjectHeaderProps {
    subject: string
    labels: { id: string; name: string; color: string }[]
}

export function ThreadSubjectHeader({ subject, labels }: ThreadSubjectHeaderProps) {
    const breakpoint = useBreakpoint()
    const isMobile = breakpoint === 'mobile'
    const theme = useTheme()

    return (
        <View style={styles.subjectRow}>
            <Text
                style={[
                    styles.subject,
                    isMobile && styles.subjectMobile,
                    { color: theme.color.val },
                ]}
            >
                {subject}
            </Text>
            <View style={styles.labelRow}>
                {labels.map(label => (
                    <LabelBadge key={label.id} name={label.name} color={label.color} />
                ))}
            </View>
        </View>
    )
}

function DeliveryStatusBadge({ status, bounceReason }: { status?: string; bounceReason?: string }) {
    const theme = useTheme()

    if (!status || status === 'sent' || status === 'delivered' || status === 'sending') return null

    const isBounce = status === 'bounced'
    const backgroundColor = isBounce ? theme.red3.val : theme.orange3.val
    const textColor = isBounce ? theme.red10.val : theme.orange10.val
    const label = isBounce ? 'Bounced' : 'Spam complaint'

    return (
        <View style={[styles.statusBadge, { backgroundColor }]}>
            <Text style={[styles.statusBadgeText, { color: textColor }]}>{label}</Text>
            {bounceReason ? (
                <Text style={[styles.statusBadgeReason, { color: textColor }]} numberOfLines={1}>
                    {bounceReason}
                </Text>
            ) : null}
        </View>
    )
}

interface MessageHeaderProps {
    senderName: string
    senderEmail: string
    date: string
    isStarred?: boolean
    deliveryStatus?: string
    bounceReason?: string
    isExpanded: boolean
    onToggleExpand: () => void
}

export function MessageHeader({
    senderName,
    senderEmail,
    date,
    isStarred,
    deliveryStatus,
    bounceReason,
    isExpanded,
    onToggleExpand,
}: MessageHeaderProps) {
    const theme = useTheme()
    const breakpoint = useBreakpoint()
    const isMobile = breakpoint === 'mobile'

    const initials = senderName
        .split(' ')
        .filter(Boolean)
        .map(n => n[0])
        .join('')
        .toUpperCase()
        .slice(0, 2)

    const dateDisplay = formatMailDate(date)

    return (
        <View style={styles.messageHeaderContainer}>
            <DeliveryStatusBadge status={deliveryStatus} bounceReason={bounceReason} />
            <Pressable onPress={onToggleExpand}>
                <View style={[styles.senderRow, { borderBottomColor: theme.borderColor.val }]}>
                    <View style={[styles.avatar, { backgroundColor: theme.accentBackground.val }]}>
                        <Text style={[styles.avatarText, { color: theme.accentColor.val }]}>
                            {initials}
                        </Text>
                    </View>
                    <View style={styles.senderInfo}>
                        <View style={styles.senderNameRow}>
                            <Text style={[styles.senderName, { color: theme.color.val }]}>
                                {senderName}
                            </Text>
                            {isMobile ? null : (
                                <Text style={[styles.senderEmail, { color: theme.color8.val }]}>
                                    {'<'}
                                    {senderEmail}
                                    {'>'}
                                </Text>
                            )}
                        </View>
                        <Text style={[styles.toLine, { color: theme.color8.val }]}>to me</Text>
                    </View>
                    <Text style={[styles.date, { color: theme.color8.val }]}>{dateDisplay}</Text>
                    {isStarred != null ? (
                        <Pressable style={styles.iconButton}>
                            <Star
                                size={18}
                                color={isStarred ? theme.yellow8.val : theme.color8.val}
                                fill={isStarred ? theme.yellow8.val : 'transparent'}
                            />
                        </Pressable>
                    ) : null}
                    <Pressable style={styles.iconButton}>
                        <Reply size={18} color={theme.color8.val} />
                    </Pressable>
                    <Pressable style={styles.iconButton}>
                        {isExpanded ? (
                            <ChevronUp size={18} color={theme.color8.val} />
                        ) : (
                            <ChevronDown size={18} color={theme.color8.val} />
                        )}
                    </Pressable>
                    <Pressable style={styles.iconButton}>
                        <MoreVertical size={18} color={theme.color8.val} />
                    </Pressable>
                </View>
            </Pressable>
        </View>
    )
}

const styles = StyleSheet.create({
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
    statusBadge: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 6,
        marginHorizontal: 16,
        marginBottom: 8,
        paddingHorizontal: 10,
        paddingVertical: 4,
        borderRadius: 4,
        alignSelf: 'flex-start',
    },
    statusBadgeText: {
        fontSize: 12,
        fontWeight: '600',
    },
    statusBadgeReason: {
        fontSize: 11,
        flexShrink: 1,
    },
    messageHeaderContainer: {
        gap: 0,
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
