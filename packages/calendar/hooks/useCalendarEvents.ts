import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import {
    createContext,
    createElement,
    type ReactNode,
    useCallback,
    useContext,
    useMemo,
    useRef,
    useState,
} from 'react'
import { useStore } from '~/lib/pocketbase'
import { useCurrentUserOrg } from '~/lib/use-current-user-org'
import { useOrgInfo } from '~/lib/use-org-info'
import { expandRecurringEvents, parseEventId } from '../lib/recurrence'
import type { CalendarEvents, CalendarWithGroup } from '../types'

interface VisibleCalendarsState {
    calendars: CalendarWithGroup[]
    mineCalendars: CalendarWithGroup[]
    otherCalendars: CalendarWithGroup[]
    visibleIds: Set<string>
    toggleCalendar: (id: string) => void
    calendarMap: Map<string, CalendarWithGroup>
    isLoading: boolean
}

const VisibleCalendarsContext = createContext<VisibleCalendarsState | null>(null)

export function VisibleCalendarsProvider({ children }: { children: ReactNode }) {
    const { orgSlug, orgId } = useOrgInfo()
    const userOrg = useCurrentUserOrg(orgSlug)
    const [calendarsCollection] = useStore('calendar_calendars')
    const [membersCollection] = useStore('calendar_members')

    const { data: allCalendars } = useLiveQuery(
        query => query.from({ cal: calendarsCollection }).where(({ cal }) => eq(cal.org, orgId)),
        [orgId]
    )

    const userOrgId = userOrg?.id ?? ''

    const { data: memberships } = useLiveQuery(
        query =>
            query.from({ mem: membersCollection }).where(({ mem }) => eq(mem.user_org, userOrgId)),
        [userOrgId]
    )

    const membershipMap = useMemo(
        () => new Map((memberships ?? []).map(m => [m.calendar, m.role])),
        [memberships]
    )

    const calendars = useMemo<CalendarWithGroup[]>(
        () =>
            (allCalendars ?? []).map(cal => ({
                ...cal,
                group: membershipMap.get(cal.id) === 'owner' ? 'mine' : 'other',
            })),
        [allCalendars, membershipMap]
    )

    const initializedRef = useRef(false)
    const [visibleIds, setVisibleIds] = useState<Set<string>>(new Set())

    if (!initializedRef.current && calendars.length > 0) {
        initializedRef.current = true
        setVisibleIds(new Set(calendars.map(c => c.id)))
    }

    const toggleCalendar = useCallback((id: string) => {
        setVisibleIds(prev => {
            const next = new Set(prev)
            if (next.has(id)) {
                next.delete(id)
            } else {
                next.add(id)
            }
            return next
        })
    }, [])

    const mineCalendars = useMemo(() => calendars.filter(c => c.group === 'mine'), [calendars])
    const otherCalendars = useMemo(() => calendars.filter(c => c.group === 'other'), [calendars])

    const calendarMap = useMemo(() => new Map(calendars.map(c => [c.id, c])), [calendars])

    const isLoading = !allCalendars || !memberships

    const value = useMemo(
        () => ({
            calendars,
            mineCalendars,
            otherCalendars,
            visibleIds,
            toggleCalendar,
            calendarMap,
            isLoading,
        }),
        [
            calendars,
            mineCalendars,
            otherCalendars,
            visibleIds,
            toggleCalendar,
            calendarMap,
            isLoading,
        ]
    )

    return createElement(VisibleCalendarsContext.Provider, { value }, children)
}

export function useVisibleCalendars(): VisibleCalendarsState {
    const ctx = useContext(VisibleCalendarsContext)
    if (!ctx) {
        throw new Error('useVisibleCalendars must be used within VisibleCalendarsProvider')
    }
    return ctx
}

export function useCalendarMap(): Map<string, CalendarWithGroup> {
    return useVisibleCalendars().calendarMap
}

export function useCalendarEvents(startDate: Date, endDate: Date) {
    const { visibleIds } = useVisibleCalendars()
    const [eventsCollection] = useStore('calendar_events')

    const { data: allEvents } = useLiveQuery(query => query.from({ evt: eventsCollection }), [])

    return useMemo(() => {
        if (!allEvents) return []
        const visible = allEvents.filter(e => visibleIds.has(e.calendar))
        return expandRecurringEvents({
            events: visible,
            rangeStart: startDate,
            rangeEnd: endDate,
        })
    }, [allEvents, startDate, endDate, visibleIds])
}

export function useEventDetail(eventId: string | undefined): {
    event: CalendarEvents | undefined
    calendar: CalendarWithGroup | undefined
    occurrenceDate?: string
} {
    const { calendarMap } = useVisibleCalendars()
    const [eventsCollection] = useStore('calendar_events')

    const { baseId, occurrenceDate } = parseEventId(eventId ?? '')

    const { data: events } = useLiveQuery(
        query => query.from({ evt: eventsCollection }).where(({ evt }) => eq(evt.id, baseId)),
        [baseId]
    )

    return useMemo(() => {
        if (!baseId || !events?.length) return { event: undefined, calendar: undefined }
        const event = events[0]
        const calendar = calendarMap.get(event.calendar)
        return { event, calendar, occurrenceDate }
    }, [baseId, events, calendarMap, occurrenceDate])
}
