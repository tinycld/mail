import { useMemo } from 'react'
import { YStack } from 'tamagui'
import { useCalendarEvents } from '../hooks/useCalendarEvents'
import { isToday } from '../hooks/useCalendarNavigation'
import { useCalendarView } from '../hooks/useCalendarView'
import { AllDayBar } from './AllDayBar'
import { DayColumnHeader } from './DayColumnHeader'
import { TimeGrid } from './TimeGrid'

export function DayView() {
    const { focusDate, openQuickCreate, openEventDetail } = useCalendarView()
    const events = useCalendarEvents(focusDate, focusDate)

    const { allDayEvents, timedEvents } = useMemo(() => {
        const allDay = events.filter(e => e.allDay)
        const timed = events.filter(e => !e.allDay)
        return { allDayEvents: allDay, timedEvents: timed }
    }, [events])

    const columns = useMemo(
        () => [{ date: focusDate, events: timedEvents }],
        [focusDate, timedEvents]
    )

    return (
        <YStack flex={1}>
            <DayColumnHeader date={focusDate} isToday={isToday(focusDate)} />
            <AllDayBar
                events={allDayEvents}
                weekStart={focusDate}
                dayCount={1}
                onEventPress={openEventDetail}
            />
            <TimeGrid
                columns={columns}
                onSlotPress={openQuickCreate}
                onEventPress={openEventDetail}
            />
        </YStack>
    )
}
