import type { ReactNode } from 'react'
import { VisibleCalendarsProvider } from './hooks/useCalendarEvents'

export default function CalendarProvider({ children }: { children: ReactNode }) {
    return <VisibleCalendarsProvider>{children}</VisibleCalendarsProvider>
}
