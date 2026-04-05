import { Clock, MapPin, Pencil, Trash2, Users, X } from 'lucide-react-native'
import { useRouter } from 'one'
import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { useOrgHref } from '~/lib/org-routes'
import { describeRRule, parseEventId } from '../lib/recurrence'
import type { CalendarEvents } from '../types'
import { getCalendarColorResolved } from './calendar-colors'
import { EventGuestList } from './EventGuestList'

interface EventDetailPopoverProps {
    isVisible: boolean
    event: CalendarEvents | undefined
    calendarName: string
    calendarColorKey: string
    onClose: () => void
    onDelete?: (eventId: string) => void
}

function formatEventDateTime(event: CalendarEvents): string {
    const start = new Date(event.start)
    const end = new Date(event.end)
    if (event.all_day) {
        return start.toLocaleDateString('en-US', {
            weekday: 'long',
            month: 'long',
            day: 'numeric',
        })
    }
    const dateStr = start.toLocaleDateString('en-US', {
        weekday: 'long',
        month: 'long',
        day: 'numeric',
    })
    const startTime = start.toLocaleTimeString('en-US', {
        hour: 'numeric',
        minute: '2-digit',
    })
    const endTime = end.toLocaleTimeString('en-US', {
        hour: 'numeric',
        minute: '2-digit',
    })
    return `${dateStr}\n${startTime} – ${endTime}`
}

function getRecurrenceLabel(event: CalendarEvents): string {
    if (!event.recurrence) return ''
    const eventStart = new Date(event.start)
    return describeRRule(event.recurrence, eventStart)
}

function MobileEventDetail({
    event,
    calendarName,
    calendarColorKey,
    onClose,
    onDelete,
}: Omit<EventDetailPopoverProps, 'isVisible'> & { event: CalendarEvents }) {
    const theme = useTheme()
    const router = useRouter()
    const orgHref = useOrgHref()
    const colors = getCalendarColorResolved(calendarColorKey)
    const dateTimeStr = formatEventDateTime(event)

    const { baseId } = parseEventId(event.id)
    const onEdit = () => {
        onClose()
        router.push(orgHref('calendar/[id]', { id: baseId }))
    }

    return (
        <View style={[mobileStyles.container, { backgroundColor: theme.background.val }]}>
            <View style={mobileStyles.topBar}>
                <Pressable onPress={onClose} hitSlop={8}>
                    <X size={22} color={theme.color.val} />
                </Pressable>
                <View style={mobileStyles.topBarActions}>
                    <Pressable onPress={onEdit} hitSlop={8}>
                        <Pencil size={20} color={theme.color8.val} />
                    </Pressable>
                    <Pressable
                        hitSlop={8}
                        onPress={() => {
                            onDelete?.(baseId)
                            onClose()
                        }}
                    >
                        <Trash2 size={20} color={theme.color8.val} />
                    </Pressable>
                </View>
            </View>

            <ScrollView style={mobileStyles.body} contentContainerStyle={mobileStyles.bodyContent}>
                <View style={mobileStyles.titleRow}>
                    <View style={[mobileStyles.colorDot, { backgroundColor: colors.bg }]} />
                    <Text style={[mobileStyles.title, { color: theme.color.val }]}>
                        {event.title}
                    </Text>
                </View>

                <View style={mobileStyles.detailRow}>
                    <Clock size={18} color={theme.color8.val} />
                    <Text style={[mobileStyles.detailText, { color: theme.color.val }]}>
                        {dateTimeStr}
                    </Text>
                </View>

                {event.recurrence ? (
                    <Text style={[mobileStyles.recurrence, { color: theme.color8.val }]}>
                        {getRecurrenceLabel(event)}
                    </Text>
                ) : null}

                {event.location ? (
                    <View style={mobileStyles.detailRow}>
                        <MapPin size={18} color={theme.color8.val} />
                        <Text style={[mobileStyles.detailText, { color: theme.color.val }]}>
                            {event.location}
                        </Text>
                    </View>
                ) : null}

                {event.guests.length > 0 ? (
                    <View style={mobileStyles.guestSection}>
                        <View style={mobileStyles.detailRow}>
                            <Users size={18} color={theme.color8.val} />
                            <Text style={[mobileStyles.detailText, { color: theme.color.val }]}>
                                {event.guests.length} guest
                                {event.guests.length !== 1 ? 's' : ''}
                            </Text>
                        </View>
                        <EventGuestList guests={event.guests} />
                    </View>
                ) : null}

                {event.description ? (
                    <Text style={[mobileStyles.description, { color: theme.color.val }]}>
                        {event.description}
                    </Text>
                ) : null}

                <Text style={[mobileStyles.calendarLabel, { color: theme.color8.val }]}>
                    {calendarName}
                </Text>
            </ScrollView>
        </View>
    )
}

export function EventDetailPopover({
    isVisible,
    event,
    calendarName,
    calendarColorKey,
    onClose,
    onDelete,
}: EventDetailPopoverProps) {
    const theme = useTheme()
    const router = useRouter()
    const orgHref = useOrgHref()
    const isMobile = useBreakpoint() === 'mobile'

    if (!isVisible || !event) return null

    if (isMobile) {
        return (
            <View style={mobileStyles.overlay}>
                <MobileEventDetail
                    event={event}
                    calendarName={calendarName}
                    calendarColorKey={calendarColorKey}
                    onClose={onClose}
                    onDelete={onDelete}
                />
            </View>
        )
    }

    const colors = getCalendarColorResolved(calendarColorKey)
    const dateTimeStr = formatEventDateTime(event)
    const { baseId: desktopBaseId } = parseEventId(event.id)

    const onEdit = () => {
        onClose()
        router.push(orgHref('calendar/[id]', { id: desktopBaseId }))
    }

    const handleDelete = () => {
        onDelete?.(parseEventId(event.id).baseId)
        onClose()
    }

    return (
        <Pressable style={styles.overlay} onPress={onClose}>
            <Pressable
                style={[
                    styles.popover,
                    {
                        backgroundColor: theme.background.val,
                        borderColor: theme.borderColor.val,
                        shadowColor: theme.shadowColor.val,
                    },
                ]}
                onPress={e => e.stopPropagation()}
            >
                <View style={styles.actions}>
                    <Pressable onPress={onEdit} hitSlop={8}>
                        <Pencil size={18} color={theme.color8.val} />
                    </Pressable>
                    <Pressable onPress={handleDelete} hitSlop={8}>
                        <Trash2 size={18} color={theme.color8.val} />
                    </Pressable>
                    <Pressable onPress={onClose} hitSlop={8}>
                        <X size={18} color={theme.color8.val} />
                    </Pressable>
                </View>

                <View style={styles.titleRow}>
                    <View style={[styles.colorBar, { backgroundColor: colors.bg }]} />
                    <Text style={[styles.title, { color: theme.color.val }]}>{event.title}</Text>
                </View>

                <View style={styles.detailRow}>
                    <Clock size={16} color={theme.color8.val} />
                    <Text style={[styles.detailText, { color: theme.color.val }]}>
                        {dateTimeStr}
                    </Text>
                </View>

                {event.location ? (
                    <View style={styles.detailRow}>
                        <MapPin size={16} color={theme.color8.val} />
                        <Text style={[styles.detailText, { color: theme.color.val }]}>
                            {event.location}
                        </Text>
                    </View>
                ) : null}

                {event.guests.length > 0 ? (
                    <View style={styles.detailRow}>
                        <Users size={16} color={theme.color8.val} />
                        <Text style={[styles.detailText, { color: theme.color.val }]}>
                            {event.guests.length} guest{event.guests.length !== 1 ? 's' : ''}
                        </Text>
                    </View>
                ) : null}

                {event.description ? (
                    <Text
                        style={[styles.description, { color: theme.color8.val }]}
                        numberOfLines={3}
                    >
                        {event.description}
                    </Text>
                ) : null}

                <Text style={[styles.calendarLabel, { color: theme.color8.val }]}>
                    {calendarName}
                </Text>
            </Pressable>
        </Pressable>
    )
}

const mobileStyles = StyleSheet.create({
    overlay: {
        position: 'absolute',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        zIndex: 100,
    },
    container: {
        flex: 1,
    },
    topBar: {
        flexDirection: 'row',
        justifyContent: 'space-between',
        alignItems: 'center',
        paddingHorizontal: 16,
        paddingVertical: 12,
    },
    topBarActions: {
        flexDirection: 'row',
        gap: 20,
    },
    body: {
        flex: 1,
    },
    bodyContent: {
        paddingHorizontal: 20,
        paddingBottom: 40,
    },
    titleRow: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 12,
        marginBottom: 20,
    },
    colorDot: {
        width: 16,
        height: 16,
        borderRadius: 8,
    },
    title: {
        fontSize: 22,
        fontWeight: '700',
        flex: 1,
    },
    detailRow: {
        flexDirection: 'row',
        alignItems: 'flex-start',
        gap: 12,
        marginBottom: 12,
    },
    detailText: {
        fontSize: 15,
        flex: 1,
    },
    recurrence: {
        fontSize: 14,
        marginBottom: 12,
        paddingLeft: 30,
    },
    guestSection: {
        marginBottom: 8,
    },
    description: {
        fontSize: 15,
        marginTop: 8,
        marginBottom: 12,
        lineHeight: 22,
    },
    calendarLabel: {
        fontSize: 13,
        marginTop: 16,
    },
})

const styles = StyleSheet.create({
    overlay: {
        position: 'absolute',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        justifyContent: 'center',
        alignItems: 'center',
        zIndex: 100,
    },
    popover: {
        width: 360,
        borderRadius: 12,
        borderWidth: 1,
        padding: 16,
        shadowOffset: { width: 0, height: 4 },
        shadowOpacity: 0.15,
        shadowRadius: 12,
        elevation: 8,
    },
    actions: {
        flexDirection: 'row',
        justifyContent: 'flex-end',
        gap: 16,
        marginBottom: 12,
    },
    titleRow: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 10,
        marginBottom: 12,
    },
    colorBar: {
        width: 4,
        height: 24,
        borderRadius: 2,
    },
    title: {
        fontSize: 18,
        fontWeight: '600',
        flex: 1,
    },
    detailRow: {
        flexDirection: 'row',
        alignItems: 'flex-start',
        gap: 10,
        marginBottom: 8,
        paddingLeft: 2,
    },
    detailText: {
        fontSize: 14,
        flex: 1,
    },
    description: {
        fontSize: 13,
        marginTop: 4,
        marginBottom: 8,
        paddingLeft: 2,
    },
    calendarLabel: {
        fontSize: 12,
        marginTop: 8,
        paddingLeft: 2,
    },
})
