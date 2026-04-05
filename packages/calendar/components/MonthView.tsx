import { useMemo } from 'react'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useCalendarEvents } from '../hooks/useCalendarEvents'
import { addDays, eventOverlapsRange } from '../hooks/useCalendarNavigation'
import { useCalendarView } from '../hooks/useCalendarView'
import { getMonthGrid, type MonthGridCell } from '../hooks/useMonthGrid'
import { type LayoutEvent, layoutMonthEvents, type MonthCellLayout } from '../layout'
import { getCalendarById } from '../mock-data'
import type { CalendarEvent } from '../types'
import { getCalendarColorResolved } from './calendar-colors'
import { MonthCell } from './MonthCell'

const DAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const MAX_VISIBLE_ROWS = 3
const MULTI_DAY_ROW_HEIGHT = 18
const DATE_HEADER_HEIGHT = 28

export function MonthView() {
    const { focusDate, openEventDetail, setViewMode, goToDate } = useCalendarView()
    const theme = useTheme()

    const grid = useMemo(() => getMonthGrid(focusDate), [focusDate])

    const gridStart = grid[0].date
    const gridEnd = grid[grid.length - 1].date

    const events = useCalendarEvents(gridStart, gridEnd)

    const rows = useMemo(() => {
        const result: MonthGridCell[][] = []
        for (let i = 0; i < grid.length; i += 7) {
            result.push(grid.slice(i, i + 7))
        }
        return result
    }, [grid])

    const eventMap = useMemo(() => new Map(events.map(e => [e.id, e])), [events])

    const rowLayouts = useMemo(
        () =>
            rows.map(row => {
                const weekStart = row[0].date
                const weekEnd = addDays(weekStart, 6)
                const weekEndFull = new Date(weekEnd)
                weekEndFull.setHours(23, 59, 59, 999)

                const weekEvents = events.filter(e => eventOverlapsRange(e, weekStart, weekEndFull))

                const layoutEvents: LayoutEvent[] = weekEvents.map(e => ({
                    id: e.id,
                    start: new Date(e.start),
                    end: new Date(e.end),
                    allDay: e.allDay,
                }))

                return layoutMonthEvents(layoutEvents, weekStart, MAX_VISIBLE_ROWS)
            }),
        [rows, events]
    )

    const handleDatePress = (date: Date) => {
        goToDate(date)
        setViewMode('day')
    }

    return (
        <View style={styles.container}>
            <View style={[styles.headerRow, { borderBottomColor: theme.borderColor.val }]}>
                {DAY_LABELS.map(label => (
                    <View key={label} style={styles.headerCell}>
                        <Text style={[styles.headerText, { color: theme.color8.val }]}>
                            {label}
                        </Text>
                    </View>
                ))}
            </View>

            {rows.map((row, rowIndex) => {
                const cellLayoutMap = rowLayouts[rowIndex]

                return (
                    <View key={row[0].date.toISOString()} style={styles.row}>
                        <MultiDayBars
                            cellLayoutMap={cellLayoutMap}
                            eventMap={eventMap}
                            onEventPress={openEventDetail}
                        />
                        {row.map((cell, colIndex) => {
                            const cellLayout = cellLayoutMap.get(colIndex)
                            return (
                                <MonthCell
                                    key={cell.date.toISOString()}
                                    date={cell.date}
                                    isCurrentMonth={cell.isCurrentMonth}
                                    isToday={cell.isToday}
                                    cellLayout={cellLayout}
                                    eventMap={eventMap}
                                    onDatePress={handleDatePress}
                                    onEventPress={openEventDetail}
                                />
                            )
                        })}
                    </View>
                )
            })}
        </View>
    )
}

interface MultiDayBarsProps {
    cellLayoutMap: Map<number, MonthCellLayout>
    eventMap: Map<string, CalendarEvent>
    onEventPress: (eventId: string) => void
}

function MultiDayBars({ cellLayoutMap, eventMap, onEventPress }: MultiDayBarsProps) {
    const rendered = new Set<string>()
    const bars: React.JSX.Element[] = []

    cellLayoutMap.forEach(cellLayout => {
        for (const layout of cellLayout.layouts) {
            if (!layout.isAllDay || rendered.has(layout.id)) continue
            rendered.add(layout.id)

            const event = eventMap.get(layout.id)
            if (!event) continue

            const cal = getCalendarById(event.calendarId)
            const colors = getCalendarColorResolved(cal?.colorKey ?? 'blue')
            const top = DATE_HEADER_HEIGHT + layout.row * MULTI_DAY_ROW_HEIGHT

            bars.push(
                <Pressable
                    key={layout.id}
                    onPress={() => onEventPress(layout.id)}
                    style={{
                        position: 'absolute',
                        top,
                        left: `${(layout.startCol / 7) * 100}%`,
                        width: `${(layout.span / 7) * 100}%`,
                        height: MULTI_DAY_ROW_HEIGHT - 2,
                        paddingHorizontal: 1,
                        zIndex: 2,
                    }}
                >
                    <View style={[styles.multiDayBar, { backgroundColor: colors.bg }]}>
                        <Text
                            style={[styles.multiDayText, { color: colors.text }]}
                            numberOfLines={1}
                        >
                            {layout.isStart ? event.title : ''}
                        </Text>
                    </View>
                </Pressable>
            )
        }
    })

    return <>{bars}</>
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
    },
    headerRow: {
        flexDirection: 'row',
        borderBottomWidth: 1,
    },
    headerCell: {
        flex: 1,
        alignItems: 'center',
        paddingVertical: 8,
    },
    headerText: {
        fontSize: 12,
        fontWeight: '600',
    },
    row: {
        flex: 1,
        flexDirection: 'row',
        position: 'relative',
    },
    multiDayBar: {
        flex: 1,
        borderRadius: 3,
        paddingHorizontal: 4,
        paddingVertical: 1,
        overflow: 'hidden',
    },
    multiDayText: {
        fontSize: 10,
        fontWeight: '600',
    },
})
