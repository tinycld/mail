import { CalendarDays, Columns3, Grid3X3, List } from 'lucide-react-native'
import { useActiveParams, useRouter } from 'one'
import { useMemo } from 'react'
import { StyleSheet, View } from 'react-native'
import {
    SidebarActionButton,
    SidebarDivider,
    SidebarItem,
    SidebarNav,
} from '~/components/sidebar-primitives'
import { useBreakpoint } from '~/components/workspace/useBreakpoint'
import { useWorkspaceLayout } from '~/components/workspace/useWorkspaceLayout'
import { useOrgHref } from '~/lib/org-routes'
import { CalendarList } from './components/CalendarList'
import { MiniCalendar } from './components/MiniCalendar'
import { useVisibleCalendars } from './hooks/useCalendarEvents'
import { parseDate, toDateString } from './hooks/useCalendarNavigation'
import type { ViewMode } from './hooks/useCalendarView'

const VIEW_MODE_OPTIONS: { mode: ViewMode; label: string; icon: typeof List }[] = [
    { mode: 'schedule', label: 'Schedule', icon: List },
    { mode: 'day', label: 'Day', icon: CalendarDays },
    { mode: 'week', label: '3 Day', icon: Columns3 },
    { mode: 'month', label: 'Month', icon: Grid3X3 },
]

interface CalendarSidebarProps {
    isCollapsed: boolean
}

export default function CalendarSidebar(props: CalendarSidebarProps) {
    return <CalendarSidebarInner {...props} />
}

function CalendarSidebarInner(_props: CalendarSidebarProps) {
    const router = useRouter()
    const orgHref = useOrgHref()
    const { calendars, visibleIds, toggleCalendar, setCalendarColor, showOnlyCalendar } =
        useVisibleCalendars()
    const { view, date } = useActiveParams<{ view?: string; date?: string }>()
    const isMobile = useBreakpoint() === 'mobile'
    const { setDrawerOpen } = useWorkspaceLayout()

    const selectedDate = useMemo(() => parseDate(date), [date])

    const handleDateSelect = (d: Date) => {
        router.push(orgHref('calendar', { view: view ?? 'week', date: toDateString(d) }))
        if (isMobile) setDrawerOpen(false)
    }

    const handleCreate = () => {
        router.push(orgHref('calendar/[id]', { id: 'new' }))
    }

    const handleViewModeSelect = (mode: ViewMode) => {
        router.push(orgHref('calendar', { view: mode, date: date ?? toDateString(new Date()) }))
        setDrawerOpen(false)
    }

    return (
        <SidebarNav>
            {isMobile && (
                <>
                    <View style={styles.viewModeSection}>
                        {VIEW_MODE_OPTIONS.map(opt => (
                            <SidebarItem
                                key={opt.mode}
                                label={opt.label}
                                icon={opt.icon}
                                isActive={view === opt.mode}
                                onPress={() => handleViewModeSelect(opt.mode)}
                            />
                        ))}
                    </View>
                    <SidebarDivider />
                </>
            )}

            {!isMobile && <SidebarActionButton label="+ Create" onPress={handleCreate} />}

            <MiniCalendar selectedDate={selectedDate} onDateSelect={handleDateSelect} />

            <SidebarDivider />

            <CalendarList
                calendars={calendars}
                visibleIds={visibleIds}
                onToggle={toggleCalendar}
                onColorChange={setCalendarColor}
                onShowOnly={showOnlyCalendar}
            />
        </SidebarNav>
    )
}

const styles = StyleSheet.create({
    viewModeSection: {
        paddingHorizontal: 8,
        paddingVertical: 4,
    },
})
