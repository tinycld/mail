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
import { useMutation } from '~/lib/mutations'
import { useStore } from '~/lib/pocketbase'
import { useCurrentUserOrg } from '~/lib/use-current-user-org'
import { useOrgInfo } from '~/lib/use-org-info'
import { expandRecurringEvents, parseEventId } from '../lib/recurrence'
import type { CalendarColorKey, CalendarEvents, CalendarWithGroup } from '../types'

interface MembershipInfo {
    id: string
    role: 'owner' | 'editor' | 'viewer'
    color: CalendarColorKey | ''
}

interface VisibleCalendarsState {
    calendars: CalendarWithGroup[]
    mineCalendars: CalendarWithGroup[]
    otherCalendars: CalendarWithGroup[]
    visibleIds: Set<string>
    toggleCalendar: (id: string) => void
    calendarMap: Map<string, CalendarWithGroup>
    membershipByCalendar: Map<string, MembershipInfo>
    setCalendarColor: (calendarId: string, color: CalendarColorKey) => void
    showOnlyCalendar: (calendarId: string) => void
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

    const membershipByCalendar = useMemo(
        () =>
            new Map(
                (memberships ?? []).map(m => [
                    m.calendar,
                    { id: m.id, role: m.role, color: m.color } as MembershipInfo,
                ])
            ),
        [memberships]
    )

    const calendars = useMemo<CalendarWithGroup[]>(
        () =>
            (allCalendars ?? []).map(cal => {
                const membership = membershipByCalendar.get(cal.id)
                return {
                    ...cal,
                    color: membership?.color || cal.color,
                    group: membership?.role === 'owner' ? 'mine' : 'other',
                }
            }),
        [allCalendars, membershipByCalendar]
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

    const showOnlyCalendar = useCallback((id: string) => {
        setVisibleIds(new Set([id]))
    }, [])

    const colorMutation = useMutation({
        mutationFn: function* ({
            membershipId,
            color,
        }: {
            membershipId: string
            color: CalendarColorKey
        }) {
            yield membersCollection.update(membershipId, draft => {
                draft.color = color
            })
        },
    })

    const setCalendarColor = useCallback(
        (calendarId: string, color: CalendarColorKey) => {
            const membership = membershipByCalendar.get(calendarId)
            if (membership) {
                colorMutation.mutate({ membershipId: membership.id, color })
            }
        },
        [membershipByCalendar, colorMutation]
    )

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
            membershipByCalendar,
            setCalendarColor,
            showOnlyCalendar,
            isLoading,
        }),
        [
            calendars,
            mineCalendars,
            otherCalendars,
            visibleIds,
            toggleCalendar,
            calendarMap,
            membershipByCalendar,
            setCalendarColor,
            showOnlyCalendar,
            isLoading,
        ]
    )

    return createElement(VisibleCalendarsContext.Provider, { value }, children)
}

const NOOP = () => {}
const EMPTY_SET = new Set<string>()
const EMPTY_MAP_CWG = new Map<string, CalendarWithGroup>()
const EMPTY_MAP_MI = new Map<string, MembershipInfo>()

const DEFAULT_STATE: VisibleCalendarsState = {
    calendars: [],
    mineCalendars: [],
    otherCalendars: [],
    visibleIds: EMPTY_SET,
    toggleCalendar: NOOP,
    calendarMap: EMPTY_MAP_CWG,
    membershipByCalendar: EMPTY_MAP_MI,
    setCalendarColor: NOOP,
    showOnlyCalendar: NOOP,
    isLoading: true,
}

export function useVisibleCalendars(): VisibleCalendarsState {
    const ctx = useContext(VisibleCalendarsContext)
    return ctx ?? DEFAULT_STATE
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
