import { Slot } from 'one'
import { YStack } from 'tamagui'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { captureException } from '~/lib/errors'
import { useMutation } from '~/lib/mutations'
import { useStore } from '~/lib/pocketbase'
import { CalendarFAB } from '../components/CalendarFAB'
import { CalendarHeader } from '../components/CalendarHeader'
import { EventDetailPopover } from '../components/EventDetailPopover'
import { EventQuickCreate } from '../components/EventQuickCreate'
import { useEventDetail, VisibleCalendarsProvider } from '../hooks/useCalendarEvents'
import { CalendarViewProvider, useCalendarView } from '../hooks/useCalendarView'

function CalendarLayoutInner() {
    const { popover, closePopover } = useCalendarView()
    const isMobile = useBreakpoint() === 'mobile'
    const [eventsCollection] = useStore('calendar_events')

    const eventId = popover.type === 'event-detail' ? popover.eventId : undefined
    const { event, calendar } = useEventDetail(eventId)

    const deleteEvent = useMutation({
        mutationFn: function* (eventId: string) {
            yield eventsCollection.delete(eventId)
        },
        onError: error => captureException('CalendarDeleteEvent', error),
    })

    return (
        <YStack flex={1} backgroundColor="$background">
            <CalendarHeader />
            <YStack flex={1}>
                <Slot />
            </YStack>

            <CalendarFAB isVisible={isMobile} />

            <EventQuickCreate
                isVisible={popover.type === 'quick-create'}
                initialDate={popover.type === 'quick-create' ? popover.date : new Date()}
                initialHour={popover.type === 'quick-create' ? popover.hour : 9}
                onClose={closePopover}
            />

            <EventDetailPopover
                isVisible={popover.type === 'event-detail'}
                event={event}
                calendarName={calendar?.name ?? ''}
                calendarColorKey={calendar?.color ?? 'blue'}
                onClose={closePopover}
                onDelete={id => deleteEvent.mutate(id)}
            />
        </YStack>
    )
}

export default function CalendarLayout() {
    return (
        <VisibleCalendarsProvider>
            <CalendarViewProvider>
                <CalendarLayoutInner />
            </CalendarViewProvider>
        </VisibleCalendarsProvider>
    )
}
