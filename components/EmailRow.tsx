import type { Href } from 'expo-router'
import { Link } from 'expo-router'
import {
    Archive,
    Inbox,
    Mail,
    MailOpen,
    Paperclip,
    Square,
    SquareCheck,
    Star,
    Trash2,
} from 'lucide-react-native'
import type { ReactNode } from 'react'
import { useState } from 'react'
import { Platform, Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { LabelBadge, LabelDots } from '~/components/LabelBadge'
import { hexToRgba } from '~/lib/color-utils'
import { formatRelativeDate } from '~/lib/format-utils'
import { useOrgHref } from '~/lib/org-routes'
import { useWebStyles } from '~/lib/use-web-styles'
import type { ThreadListItem } from './thread-list-item'

const tooltipCSS = `
    .hover-action-tooltip {
        position: relative;
        display: inline-flex;
    }
    .hover-action-tooltip::after {
        content: attr(data-tooltip);
        position: absolute;
        left: 50%;
        transform: translateX(-50%);
        padding: 4px 8px;
        border-radius: 4px;
        font-size: 12px;
        line-height: 1;
        white-space: nowrap;
        pointer-events: none;
        opacity: 0;
        transition: opacity 0.15s ease-in;
        background: var(--tooltip-bg);
        color: var(--tooltip-fg);
        z-index: 10;
    }
    .hover-action-tooltip.tooltip-above::after {
        bottom: calc(100% + 6px);
    }
    .hover-action-tooltip.tooltip-below::after {
        top: calc(100% + 6px);
    }
    .hover-action-tooltip:hover::after {
        opacity: 1;
    }
`

interface EmailRowProps {
    email: ThreadListItem
    isMobile?: boolean
    onToggleStar?: () => void
    isSelected?: boolean
    onToggleSelect?: () => void
    onPress?: () => void
    onArchive?: () => void
    onTrash?: () => void
    onToggleRead?: () => void
    index?: number
}

export function EmailRow({
    email,
    isMobile,
    onToggleStar,
    isSelected,
    onToggleSelect,
    onPress,
    onArchive,
    onTrash,
    onToggleRead,
    index,
}: EmailRowProps) {
    if (isMobile)
        return <MobileEmailRow email={email} onToggleStar={onToggleStar} onPress={onPress} />
    return (
        <DesktopEmailRow
            email={email}
            onToggleStar={onToggleStar}
            isSelected={isSelected}
            onToggleSelect={onToggleSelect}
            onPress={onPress}
            onArchive={onArchive}
            onTrash={onTrash}
            onToggleRead={onToggleRead}
            index={index}
        />
    )
}

function RowWrapper({
    href,
    onPress,
    children,
}: {
    href: Href
    onPress?: () => void
    children: ReactNode
}) {
    if (onPress) {
        return (
            <Pressable onPress={onPress} style={{ display: 'flex', width: '100%' }}>
                {children}
            </Pressable>
        )
    }
    return (
        <Link href={href} style={{ display: 'flex', width: '100%' }}>
            {children}
        </Link>
    )
}

function SenderWithDraft({
    email,
    theme,
    style,
    numberOfLines,
}: {
    email: ThreadListItem
    theme: ReturnType<typeof useTheme>
    style: Record<string, unknown>[]
    numberOfLines?: number
}) {
    if (!email.hasDraft) {
        return (
            <Text style={style} numberOfLines={numberOfLines}>
                {email.senderName}
            </Text>
        )
    }

    const otherParticipants = email.participants
        .filter(p => p.email !== email.senderEmail)
        .map(p => p.name || p.email.split('@')[0])

    if (otherParticipants.length === 0) {
        return (
            <Text style={style} numberOfLines={numberOfLines}>
                <Text style={{ color: theme.red10.val }}>Draft</Text>
            </Text>
        )
    }

    return (
        <Text style={style} numberOfLines={numberOfLines}>
            {otherParticipants.join(', ')}, <Text style={{ color: theme.red10.val }}>Draft</Text>
        </Text>
    )
}

function HoverAction({
    icon: Icon,
    label,
    onPress,
    theme,
    tooltipPosition = 'above',
}: {
    icon: typeof Archive
    label: string
    onPress?: () => void
    theme: ReturnType<typeof useTheme>
    tooltipPosition?: 'above' | 'below'
}) {
    const button = (
        <Pressable
            style={hoverActionStyles.button}
            onPress={e => {
                e.stopPropagation()
                e.preventDefault()
                onPress?.()
            }}
            accessibilityLabel={label}
        >
            <Icon size={16} color={theme.color8.val} />
        </Pressable>
    )

    if (Platform.OS !== 'web') return button

    const tooltipStyle = {
        '--tooltip-bg': theme.color2.val,
        '--tooltip-fg': theme.color12.val,
    }

    return (
        <div
            data-tooltip={label}
            className={`hover-action-tooltip tooltip-${tooltipPosition}`}
            style={tooltipStyle as never}
        >
            {button}
        </div>
    )
}

const hoverActionStyles = StyleSheet.create({
    button: {
        padding: 6,
        borderRadius: 16,
    },
})

function MobileEmailRow({ email, onToggleStar, onPress }: EmailRowProps) {
    const theme = useTheme()
    const orgHref = useOrgHref()

    const senderWeight = email.isRead ? ('400' as const) : ('700' as const)
    const subjectWeight = email.isRead ? ('400' as const) : ('600' as const)
    const rowBg = email.isRead ? 'transparent' : theme.backgroundHover.val

    const displayName = email.hasDraft ? 'Draft' : email.senderName
    const initials = displayName
        .split(' ')
        .filter(Boolean)
        .map(n => n[0])
        .join('')
        .toUpperCase()
        .slice(0, 2)

    const dateDisplay = formatRelativeDate(email.latestDate)

    return (
        <RowWrapper href={orgHref('mail/[id]', { id: email.threadId })} onPress={onPress}>
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
                        <SenderWithDraft
                            email={email}
                            theme={theme}
                            style={[
                                mobileStyles.sender,
                                { color: theme.color.val, fontWeight: senderWeight },
                            ]}
                            numberOfLines={1}
                        />
                        {email.hasAttachments ? (
                            <Paperclip size={14} color={theme.color8.val} />
                        ) : null}
                        <Text style={[mobileStyles.date, { color: theme.color8.val }]}>
                            {dateDisplay}
                        </Text>
                        <Pressable
                            style={mobileStyles.starButton}
                            onPress={e => {
                                e.stopPropagation()
                                onToggleStar?.()
                            }}
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
                        {email.snippet}
                    </Text>
                    {email.labels.length > 0 ? (
                        <View style={mobileStyles.labelRow}>
                            {email.labels.slice(0, 3).map(label => (
                                <LabelBadge key={label.id} name={label.name} color={label.color} />
                            ))}
                        </View>
                    ) : null}
                </View>
            </View>
        </RowWrapper>
    )
}

function RowHoverActions({
    email,
    isSelected,
    onArchive,
    onTrash,
    onToggleRead,
    tooltipPosition = 'above',
}: Pick<EmailRowProps, 'email' | 'isSelected' | 'onArchive' | 'onTrash' | 'onToggleRead'> & {
    tooltipPosition?: 'above' | 'below'
}) {
    const theme = useTheme()
    const ReadIcon = email.isRead ? Mail : MailOpen
    const bg = isSelected ? hexToRgba(theme.accentBackground.val, 0.09) : theme.background.val

    const isTrashed = email.folder === 'trash'
    const isArchived = email.folder === 'archive'
    const ArchiveIcon = isArchived ? Inbox : Archive
    const archiveLabel = isArchived ? 'Move to Inbox' : 'Archive'

    return (
        <View style={[styles.hoverActions, { backgroundColor: bg }]}>
            <HoverAction
                icon={ArchiveIcon}
                label={archiveLabel}
                onPress={onArchive}
                theme={theme}
                tooltipPosition={tooltipPosition}
            />
            <HoverAction
                icon={isTrashed ? Inbox : Trash2}
                label={isTrashed ? 'Move to Inbox' : 'Delete'}
                onPress={onTrash}
                theme={theme}
                tooltipPosition={tooltipPosition}
            />
            <HoverAction
                icon={ReadIcon}
                label={email.isRead ? 'Mark as unread' : 'Mark as read'}
                onPress={onToggleRead}
                theme={theme}
                tooltipPosition={tooltipPosition}
            />
        </View>
    )
}

function DesktopEmailRow({
    email,
    onToggleStar,
    isSelected,
    onToggleSelect,
    onPress,
    onArchive,
    onTrash,
    onToggleRead,
    index,
}: EmailRowProps) {
    useWebStyles('hover-action-tooltip', tooltipCSS)
    const theme = useTheme()
    const orgHref = useOrgHref()
    const [isHovered, setIsHovered] = useState(false)

    const rowBg = isSelected
        ? hexToRgba(theme.accentBackground.val, 0.09)
        : email.isRead
          ? 'transparent'
          : theme.backgroundHover.val
    const senderWeight = email.isRead ? '400' : '700'
    const subjectWeight = email.isRead ? '400' : '600'
    const dateDisplay = formatRelativeDate(email.latestDate)

    const CheckboxIcon = isSelected ? SquareCheck : Square
    const checkboxColor = isSelected ? theme.accentBackground.val : theme.color8.val

    const hoverWebProps =
        Platform.OS === 'web'
            ? {
                  onMouseEnter: () => setIsHovered(true),
                  onMouseLeave: () => setIsHovered(false),
              }
            : {}

    const borderInset = hexToRgba(theme.borderColor.val, 0.6)
    const hoverShadow =
        isHovered && Platform.OS === 'web'
            ? ({
                  boxShadow: `0 2px 6px 0 rgba(0,0,0,0.12), inset 1px 0 0 ${borderInset}, inset -1px 0 0 ${borderInset}`,
                  zIndex: 2,
              } as Record<string, unknown>)
            : null

    return (
        <RowWrapper href={orgHref('mail/[id]', { id: email.threadId })} onPress={onPress}>
            <View
                style={[
                    styles.row,
                    {
                        backgroundColor: rowBg,
                        borderBottomColor: theme.borderColor.val,
                    },
                    hoverShadow,
                ]}
                {...hoverWebProps}
            >
                <Pressable
                    style={styles.checkbox}
                    onPress={e => {
                        e.stopPropagation()
                        e.preventDefault()
                        onToggleSelect?.()
                    }}
                >
                    <CheckboxIcon size={16} color={checkboxColor} />
                </Pressable>
                <Pressable
                    style={styles.starButton}
                    onPress={e => {
                        e.stopPropagation()
                        onToggleStar?.()
                    }}
                >
                    <Star
                        size={16}
                        color={email.isStarred ? theme.yellow8.val : theme.color8.val}
                        fill={email.isStarred ? theme.yellow8.val : 'transparent'}
                    />
                </Pressable>
                <SenderWithDraft
                    email={email}
                    theme={theme}
                    style={[
                        styles.sender,
                        { color: theme.color.val, fontWeight: senderWeight as '400' | '700' },
                    ]}
                    numberOfLines={1}
                />
                {email.messageCount > 1 ? (
                    <Text style={[styles.threadBadge, { color: theme.color8.val }]}>
                        {email.messageCount}
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
                        {email.snippet}
                    </Text>
                </View>
                {email.labels.length > 0 ? <LabelDots labels={email.labels} max={3} /> : null}
                {email.hasAttachments ? <Paperclip size={14} color={theme.color8.val} /> : null}
                {isHovered ? (
                    <RowHoverActions
                        email={email}
                        isSelected={isSelected}
                        onArchive={onArchive}
                        onTrash={onTrash}
                        onToggleRead={onToggleRead}
                        tooltipPosition={index === 0 ? 'below' : 'above'}
                    />
                ) : (
                    <Text style={[styles.date, { color: theme.color8.val }]}>{dateDisplay}</Text>
                )}
            </View>
        </RowWrapper>
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
        flex: 1,
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
    labelRow: {
        flexDirection: 'row',
        flexWrap: 'wrap',
        gap: 4,
        marginTop: 2,
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
        flex: 1,
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
    hoverActions: {
        flexDirection: 'row',
        alignItems: 'center',
        marginLeft: 8,
        flexShrink: 0,
    },
})
