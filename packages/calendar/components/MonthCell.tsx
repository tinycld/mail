import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useCalendarMap } from '../hooks/useCalendarEvents'
import { formatShortTime } from '../hooks/useCalendarNavigation'
import type { MonthCellLayout } from '../layout'
import type { CalendarEvents } from '../types'
import { getCalendarColorResolved } from './calendar-colors'

interface MonthCellProps {
    date: Date
    isCurrentMonth: boolean
    isToday: boolean
    cellLayout: MonthCellLayout | undefined
    eventMap: Map<string, CalendarEvents>
    onDatePress: (date: Date) => void
    onEventPress: (eventId: string) => void
}

export function MonthCell({
    date,
    isCurrentMonth,
    isToday,
    cellLayout,
    eventMap,
    onDatePress,
    onEventPress,
}: MonthCellProps) {
    const theme = useTheme()
    const calendarMap = useCalendarMap()
    const dateNum = date.getDate()
    const layouts = cellLayout?.layouts ?? []
    const overflowCount = cellLayout?.overflowCount ?? 0

    return (
        <Pressable
            style={[styles.cell, { borderColor: theme.borderColor.val }]}
            onPress={() => onDatePress(date)}
        >
            <View
                style={[
                    styles.dateCircle,
                    isToday && {
                        backgroundColor: theme.accentBackground.val,
                    },
                ]}
            >
                <Text
                    style={[
                        styles.dateText,
                        {
                            color: isToday
                                ? theme.accentColor.val
                                : isCurrentMonth
                                  ? theme.color.val
                                  : theme.color8.val,
                        },
                    ]}
                >
                    {dateNum}
                </Text>
            </View>

            {layouts.map(layout => {
                if (layout.isAllDay) return null

                const event = eventMap.get(layout.id)
                if (!event) return null

                const cal = calendarMap.get(event.calendar)
                const colors = getCalendarColorResolved(cal?.color ?? 'blue')

                if (event.all_day) {
                    return (
                        <Pressable key={event.id} onPress={() => onEventPress(event.id)}>
                            <View style={[styles.allDayPill, { backgroundColor: colors.bg }]}>
                                <Text
                                    style={[styles.allDayText, { color: colors.text }]}
                                    numberOfLines={1}
                                >
                                    {event.title}
                                </Text>
                            </View>
                        </Pressable>
                    )
                }

                const timeStr = formatShortTime(new Date(event.start))

                return (
                    <Pressable key={event.id} onPress={() => onEventPress(event.id)}>
                        <View style={styles.timedEvent}>
                            <View style={[styles.eventDot, { backgroundColor: colors.bg }]} />
                            <Text style={[styles.eventTime, { color: theme.color8.val }]}>
                                {timeStr}
                            </Text>
                            <Text
                                style={[styles.eventTitle, { color: theme.color.val }]}
                                numberOfLines={1}
                            >
                                {event.title}
                            </Text>
                        </View>
                    </Pressable>
                )
            })}

            {overflowCount > 0 && (
                <Pressable onPress={() => onDatePress(date)}>
                    <Text style={[styles.moreText, { color: theme.color8.val }]}>
                        +{overflowCount} more
                    </Text>
                </Pressable>
            )}
        </Pressable>
    )
}

const styles = StyleSheet.create({
    cell: {
        flex: 1,
        borderRightWidth: StyleSheet.hairlineWidth,
        borderBottomWidth: StyleSheet.hairlineWidth,
        padding: 2,
        overflow: 'hidden',
    },
    dateCircle: {
        width: 24,
        height: 24,
        borderRadius: 12,
        alignItems: 'center',
        justifyContent: 'center',
        alignSelf: 'flex-end',
        marginBottom: 2,
        marginRight: 2,
    },
    dateText: {
        fontSize: 12,
        fontWeight: '500',
    },
    allDayPill: {
        borderRadius: 3,
        paddingHorizontal: 4,
        paddingVertical: 1,
        marginBottom: 1,
    },
    allDayText: {
        fontSize: 11,
        fontWeight: '600',
    },
    timedEvent: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 3,
        paddingVertical: 1,
    },
    eventDot: {
        width: 6,
        height: 6,
        borderRadius: 3,
    },
    eventTime: {
        fontSize: 10,
    },
    eventTitle: {
        fontSize: 11,
        flex: 1,
    },
    moreText: {
        fontSize: 11,
        fontWeight: '600',
        paddingVertical: 2,
        textAlign: 'center',
    },
})
