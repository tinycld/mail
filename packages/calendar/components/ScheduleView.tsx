import { useMemo } from 'react'
import { FlatList, Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useCalendarEvents, useCalendarMap } from '../hooks/useCalendarEvents'
import {
    addDays,
    isToday as checkIsToday,
    eventOverlapsRange,
    getShortDayName,
    toDateString,
} from '../hooks/useCalendarNavigation'
import { useCalendarView } from '../hooks/useCalendarView'
import type { CalendarEvents } from '../types'
import { getCalendarColorResolved } from './calendar-colors'

const SCHEDULE_DAYS = 30

interface DayRow {
    key: string
    date: Date
    events: CalendarEvents[]
    today: boolean
}

function formatTimeRange(event: CalendarEvents): string {
    if (event.all_day) return 'All day'
    const start = new Date(event.start)
    const end = new Date(event.end)
    const fmt = (d: Date) => d.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })
    return `${fmt(start)} – ${fmt(end)}`
}

function EventCard({ event, onPress }: { event: CalendarEvents; onPress: (id: string) => void }) {
    const theme = useTheme()
    const calendarMap = useCalendarMap()
    const cal = calendarMap.get(event.calendar)
    const colors = getCalendarColorResolved(cal?.color ?? 'blue')

    return (
        <Pressable
            style={[styles.eventCard, { backgroundColor: theme.color2.val }]}
            onPress={() => onPress(event.id)}
        >
            <View style={[styles.colorStrip, { backgroundColor: colors.bg }]} />
            <View style={styles.eventContent}>
                <Text style={[styles.eventTitle, { color: theme.color.val }]} numberOfLines={1}>
                    {event.title}
                </Text>
                <Text style={[styles.eventTime, { color: theme.color8.val }]}>
                    {formatTimeRange(event)}
                </Text>
                {event.location ? (
                    <Text
                        style={[styles.eventLocation, { color: theme.color8.val }]}
                        numberOfLines={1}
                    >
                        {event.location}
                    </Text>
                ) : null}
            </View>
        </Pressable>
    )
}

function DaySection({
    row,
    onEventPress,
    onEmptyPress,
}: {
    row: DayRow
    onEventPress: (id: string) => void
    onEmptyPress: (date: Date) => void
}) {
    const theme = useTheme()

    return (
        <View style={[styles.dayRow, { borderBottomColor: theme.borderColor.val }]}>
            <View style={styles.dateSide}>
                <Text style={[styles.dayName, row.today && { color: theme.accentBackground.val }]}>
                    {getShortDayName(row.date)}
                </Text>
                <View
                    style={[
                        styles.dateCircle,
                        row.today && { backgroundColor: theme.accentBackground.val },
                    ]}
                >
                    <Text
                        style={[
                            styles.dateNum,
                            { color: row.today ? theme.background.val : theme.color.val },
                        ]}
                    >
                        {row.date.getDate()}
                    </Text>
                </View>
            </View>
            <View style={styles.eventsSide}>
                {row.events.length > 0 ? (
                    row.events.map(event => (
                        <EventCard key={event.id} event={event} onPress={onEventPress} />
                    ))
                ) : (
                    <Pressable onPress={() => onEmptyPress(row.date)}>
                        <Text style={[styles.emptyText, { color: theme.color8.val }]}>
                            Nothing planned
                        </Text>
                    </Pressable>
                )}
            </View>
        </View>
    )
}

export function ScheduleView() {
    const { focusDate, openQuickCreate, openEventDetail } = useCalendarView()
    const endDate = useMemo(() => addDays(focusDate, SCHEDULE_DAYS - 1), [focusDate])
    const events = useCalendarEvents(focusDate, endDate)

    const rows = useMemo(() => {
        const result: DayRow[] = []
        for (let i = 0; i < SCHEDULE_DAYS; i++) {
            const date = addDays(focusDate, i)
            const dayStart = new Date(date)
            dayStart.setHours(0, 0, 0, 0)
            const dayEnd = new Date(date)
            dayEnd.setHours(23, 59, 59, 999)
            const dayEvents = events
                .filter(e => eventOverlapsRange(e, dayStart, dayEnd))
                .sort((a, b) => {
                    if (a.all_day && !b.all_day) return -1
                    if (!a.all_day && b.all_day) return 1
                    return new Date(a.start).getTime() - new Date(b.start).getTime()
                })
            result.push({
                key: toDateString(date),
                date,
                events: dayEvents,
                today: checkIsToday(date),
            })
        }
        return result
    }, [focusDate, events])

    const handleEmptyPress = (date: Date) => {
        openQuickCreate(date, 9)
    }

    return (
        <FlatList
            data={rows}
            keyExtractor={row => row.key}
            renderItem={({ item }) => (
                <DaySection
                    row={item}
                    onEventPress={openEventDetail}
                    onEmptyPress={handleEmptyPress}
                />
            )}
            style={styles.list}
        />
    )
}

const styles = StyleSheet.create({
    list: {
        flex: 1,
    },
    dayRow: {
        flexDirection: 'row',
        borderBottomWidth: StyleSheet.hairlineWidth,
        paddingVertical: 12,
        paddingHorizontal: 16,
        gap: 16,
    },
    dateSide: {
        width: 44,
        alignItems: 'center',
    },
    dayName: {
        fontSize: 11,
        fontWeight: '600',
        textTransform: 'uppercase',
        marginBottom: 2,
    },
    dateCircle: {
        width: 32,
        height: 32,
        borderRadius: 16,
        alignItems: 'center',
        justifyContent: 'center',
    },
    dateNum: {
        fontSize: 18,
        fontWeight: '500',
    },
    eventsSide: {
        flex: 1,
        gap: 6,
        justifyContent: 'center',
    },
    eventCard: {
        flexDirection: 'row',
        borderRadius: 8,
        overflow: 'hidden',
    },
    colorStrip: {
        width: 4,
    },
    eventContent: {
        flex: 1,
        paddingVertical: 8,
        paddingHorizontal: 10,
    },
    eventTitle: {
        fontSize: 14,
        fontWeight: '600',
        marginBottom: 2,
    },
    eventTime: {
        fontSize: 12,
    },
    eventLocation: {
        fontSize: 12,
        marginTop: 1,
    },
    emptyText: {
        fontSize: 13,
        fontStyle: 'italic',
    },
})
