import { useMemo } from 'react'
import { StyleSheet, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { useCalendarEvents } from '../hooks/useCalendarEvents'
import { addDays, eventOverlapsRange, isToday, startOfWeek } from '../hooks/useCalendarNavigation'
import { useCalendarView } from '../hooks/useCalendarView'
import type { CalendarEvents } from '../types'
import { AllDayBar } from './AllDayBar'
import { DayColumnHeader } from './DayColumnHeader'
import { TimeGrid } from './TimeGrid'

function getEventsForDay(events: CalendarEvents[], date: Date): CalendarEvents[] {
    const dayStart = new Date(date)
    dayStart.setHours(0, 0, 0, 0)
    const dayEnd = new Date(date)
    dayEnd.setHours(23, 59, 59, 999)

    return events.filter(event => eventOverlapsRange(event, dayStart, dayEnd))
}

export function WeekView() {
    const { focusDate, openQuickCreate, openEventDetail } = useCalendarView()
    const theme = useTheme()
    const isMobile = useBreakpoint() === 'mobile'
    const dayCount = isMobile ? 3 : 7

    const rangeStart = useMemo(
        () => (isMobile ? focusDate : startOfWeek(focusDate)),
        [focusDate, isMobile]
    )
    const rangeEnd = useMemo(() => addDays(rangeStart, dayCount - 1), [rangeStart, dayCount])
    const days = useMemo(
        () => Array.from({ length: dayCount }, (_, i) => addDays(rangeStart, i)),
        [rangeStart, dayCount]
    )

    const events = useCalendarEvents(rangeStart, rangeEnd)

    const { allDayEvents, columns } = useMemo(() => {
        const allDay = events.filter(e => e.all_day)
        const timed = events.filter(e => !e.all_day)
        const cols = days.map(date => ({
            date,
            events: getEventsForDay(timed, date),
        }))
        return { allDayEvents: allDay, columns: cols }
    }, [events, days])

    return (
        <View style={styles.container}>
            <View style={[styles.headerRow, { borderBottomColor: theme.borderColor.val }]}>
                <View style={styles.gutterSpacer} />
                {days.map(date => (
                    <View key={date.toISOString()} style={styles.headerCell}>
                        <DayColumnHeader date={date} isToday={isToday(date)} />
                    </View>
                ))}
            </View>
            <AllDayBar
                events={allDayEvents}
                weekStart={rangeStart}
                dayCount={dayCount}
                onEventPress={openEventDetail}
            />
            <TimeGrid
                columns={columns}
                onSlotPress={openQuickCreate}
                onEventPress={openEventDetail}
            />
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
    },
    headerRow: {
        flexDirection: 'row',
        borderBottomWidth: 1,
    },
    gutterSpacer: {
        width: 50,
    },
    headerCell: {
        flex: 1,
        alignItems: 'center',
    },
})
