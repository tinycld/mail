import type { Orgs, UserOrg } from '~/types/pbSchema'

export type CalendarColorKey = 'blue' | 'green' | 'red' | 'teal' | 'purple' | 'orange'

export type Recurrence = '' | 'daily' | 'weekly' | 'monthly' | 'yearly'

export interface EventGuest {
    name: string
    email: string
    rsvp: 'accepted' | 'declined' | 'tentative' | 'pending'
    role: 'organizer' | 'attendee'
}

export interface CalendarCalendars {
    id: string
    org: string
    name: string
    description: string
    color: CalendarColorKey
    created: string
    updated: string
}

export interface CalendarMembers {
    id: string
    calendar: string
    user_org: string
    role: 'owner' | 'editor' | 'viewer'
    created: string
    updated: string
}

export interface CalendarEvents {
    id: string
    calendar: string
    created_by: string
    title: string
    description: string
    location: string
    start: string
    end: string
    all_day: boolean
    recurrence: string
    guests: EventGuest[]
    reminder: number
    busy_status: 'busy' | 'free'
    visibility: 'default' | 'public' | 'private'
    ical_uid: string
    created: string
    updated: string
}

export type CalendarWithGroup = CalendarCalendars & { group: 'mine' | 'other' }

export type CalendarSchema = {
    calendar_calendars: {
        type: CalendarCalendars
        relations: {
            org: Orgs
        }
    }
    calendar_members: {
        type: CalendarMembers
        relations: {
            calendar: CalendarCalendars
            user_org: UserOrg
        }
    }
    calendar_events: {
        type: CalendarEvents
        relations: {
            calendar: CalendarCalendars
            created_by: UserOrg
        }
    }
}
