import { getTokenValue } from 'tamagui'
import type { CalendarColorKey } from '../types'

const CALENDAR_COLORS: Record<CalendarColorKey, { bg: string; text: string }> = {
    blue: { bg: '$blue10', text: '$white1' },
    green: { bg: '$green10', text: '$white1' },
    red: { bg: '$red10', text: '$white1' },
    teal: { bg: '$teal10', text: '$white1' },
    purple: { bg: '$purple10', text: '$white1' },
    orange: { bg: '$orange10', text: '$white1' },
    tomato: { bg: '#D50000', text: '#ffffff' },
    flamingo: { bg: '#E67C73', text: '#ffffff' },
    tangerine: { bg: '#F4511E', text: '#ffffff' },
    banana: { bg: '#E4C441', text: '#ffffff' },
    sage: { bg: '#33B679', text: '#ffffff' },
    basil: { bg: '#0B8043', text: '#ffffff' },
    peacock: { bg: '#039BE5', text: '#ffffff' },
    blueberry: { bg: '#3F51B5', text: '#ffffff' },
    lavender: { bg: '#7986CB', text: '#ffffff' },
    grape: { bg: '#8E24AA', text: '#ffffff' },
    graphite: { bg: '#616161', text: '#ffffff' },
}

export const CALENDAR_COLOR_KEYS = Object.keys(CALENDAR_COLORS) as CalendarColorKey[]

export const CALENDAR_COLOR_GRID: CalendarColorKey[][] = [
    ['tomato', 'flamingo', 'tangerine', 'banana', 'sage', 'basil'],
    ['peacock', 'blueberry', 'lavender', 'grape', 'graphite'],
    ['blue', 'green', 'red', 'teal', 'purple', 'orange'],
]

const DEFAULT_COLOR: CalendarColorKey = 'blue'

export function getCalendarColor(colorKey: string) {
    return CALENDAR_COLORS[colorKey as CalendarColorKey] ?? CALENDAR_COLORS[DEFAULT_COLOR]
}

function resolveColor(value: string, fallback: string) {
    if (!value.startsWith('$')) return value
    const resolved = getTokenValue(value as never, 'color')
    return typeof resolved === 'string' ? resolved : fallback
}

export function getCalendarColorResolved(colorKey: string) {
    const tokens = getCalendarColor(colorKey)
    return {
        bg: resolveColor(tokens.bg, '#3b82f6'),
        text: resolveColor(tokens.text, '#ffffff'),
    }
}
