import { Square, Star } from 'lucide-react-native'
import type { OneRouter } from 'one'
import { Link } from 'one'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import type { MockEmail } from './mockData'

interface EmailRowProps {
    email: MockEmail
    isMobile?: boolean
}

export function EmailRow({ email, isMobile }: EmailRowProps) {
    if (isMobile) return <MobileEmailRow email={email} />
    return <DesktopEmailRow email={email} />
}

function MobileEmailRow({ email }: EmailRowProps) {
    const theme = useTheme()

    const senderWeight = email.isRead ? ('400' as const) : ('700' as const)
    const subjectWeight = email.isRead ? ('400' as const) : ('600' as const)
    const rowBg = email.isRead ? 'transparent' : theme.backgroundHover.val

    const initials = email.sender
        .split(' ')
        .map(n => n[0])
        .join('')
        .toUpperCase()
        .slice(0, 2)

    return (
        <Link href={`/app/mail/${email.id}` as OneRouter.Href}>
            <View
                style={[
                    mobileStyles.row,
                    {
                        backgroundColor: rowBg,
                        borderBottomColor: theme.borderColor.val,
                    },
                ]}
            >
                <View
                    style={[mobileStyles.avatar, { backgroundColor: theme.accentBackground.val }]}
                >
                    <Text style={[mobileStyles.avatarText, { color: theme.accentColor.val }]}>
                        {initials}
                    </Text>
                </View>
                <View style={mobileStyles.content}>
                    <View style={mobileStyles.topRow}>
                        <Text
                            style={[
                                mobileStyles.sender,
                                { color: theme.color.val, fontWeight: senderWeight },
                            ]}
                            numberOfLines={1}
                        >
                            {email.sender}
                        </Text>
                        <Text style={[mobileStyles.date, { color: theme.color8.val }]}>
                            {email.date}
                        </Text>
                        <Pressable
                            style={mobileStyles.starButton}
                            onPress={e => e.stopPropagation()}
                        >
                            <Star
                                size={16}
                                color={email.isStarred ? theme.yellow8.val : theme.color8.val}
                                fill={email.isStarred ? theme.yellow8.val : 'transparent'}
                            />
                        </Pressable>
                    </View>
                    <Text
                        style={[
                            mobileStyles.subject,
                            { color: theme.color.val, fontWeight: subjectWeight },
                        ]}
                        numberOfLines={1}
                    >
                        {email.subject}
                    </Text>
                    <Text
                        style={[mobileStyles.preview, { color: theme.color8.val }]}
                        numberOfLines={2}
                    >
                        {email.preview}
                    </Text>
                </View>
            </View>
        </Link>
    )
}

function DesktopEmailRow({ email }: EmailRowProps) {
    const theme = useTheme()

    const rowBg = email.isRead ? 'transparent' : theme.backgroundHover.val
    const senderWeight = email.isRead ? '400' : '700'
    const subjectWeight = email.isRead ? '400' : '600'

    return (
        <Link href={`/app/mail/${email.id}` as OneRouter.Href}>
            <View
                style={[
                    styles.row,
                    {
                        backgroundColor: rowBg,
                        borderBottomColor: theme.borderColor.val,
                    },
                ]}
            >
                <Pressable style={styles.checkbox} onPress={e => e.stopPropagation()}>
                    <Square size={16} color={theme.color8.val} />
                </Pressable>
                <Pressable style={styles.starButton} onPress={e => e.stopPropagation()}>
                    <Star
                        size={16}
                        color={email.isStarred ? theme.yellow8.val : theme.color8.val}
                        fill={email.isStarred ? theme.yellow8.val : 'transparent'}
                    />
                </Pressable>
                <Text
                    style={[
                        styles.sender,
                        { color: theme.color.val, fontWeight: senderWeight as '400' | '700' },
                    ]}
                    numberOfLines={1}
                >
                    {email.sender}
                </Text>
                {email.threadCount && email.threadCount > 1 ? (
                    <Text style={[styles.threadBadge, { color: theme.color8.val }]}>
                        {email.threadCount}
                    </Text>
                ) : null}
                <View style={styles.subjectArea}>
                    <Text
                        style={[
                            styles.subject,
                            { color: theme.color.val, fontWeight: subjectWeight as '400' | '600' },
                        ]}
                        numberOfLines={1}
                    >
                        {email.subject}
                    </Text>
                    <Text style={[styles.preview, { color: theme.color8.val }]} numberOfLines={1}>
                        {' \u2014 '}
                        {email.preview}
                    </Text>
                </View>
                <Text style={[styles.date, { color: theme.color8.val }]}>{email.date}</Text>
            </View>
        </Link>
    )
}

const mobileStyles = StyleSheet.create({
    row: {
        flexDirection: 'row',
        alignItems: 'flex-start',
        paddingHorizontal: 12,
        paddingVertical: 12,
        borderBottomWidth: 1,
        gap: 12,
    },
    avatar: {
        width: 40,
        height: 40,
        borderRadius: 20,
        alignItems: 'center',
        justifyContent: 'center',
        flexShrink: 0,
    },
    avatarText: {
        fontSize: 14,
        fontWeight: '600',
    },
    content: {
        flex: 1,
        gap: 2,
    },
    topRow: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 4,
    },
    sender: {
        fontSize: 14,
        flex: 1,
    },
    date: {
        fontSize: 12,
        flexShrink: 0,
    },
    starButton: {
        padding: 2,
        flexShrink: 0,
    },
    subject: {
        fontSize: 13,
    },
    preview: {
        fontSize: 13,
        lineHeight: 18,
    },
})

const styles = StyleSheet.create({
    row: {
        flexDirection: 'row',
        alignItems: 'center',
        height: 40,
        paddingHorizontal: 8,
        borderBottomWidth: 1,
        gap: 4,
    },
    checkbox: {
        padding: 4,
    },
    starButton: {
        padding: 4,
    },
    sender: {
        fontSize: 13,
        width: 140,
        flexShrink: 0,
    },
    threadBadge: {
        fontSize: 11,
        flexShrink: 0,
    },
    subjectArea: {
        flex: 1,
        flexDirection: 'row',
        alignItems: 'center',
        overflow: 'hidden',
    },
    subject: {
        fontSize: 13,
        flexShrink: 0,
    },
    preview: {
        fontSize: 13,
        flex: 1,
    },
    date: {
        fontSize: 12,
        flexShrink: 0,
        marginLeft: 8,
    },
})
