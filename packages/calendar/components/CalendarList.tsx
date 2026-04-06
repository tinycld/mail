import { Check, ChevronDown, ChevronRight } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import type { CalendarColorKey, CalendarWithGroup } from '../types'
import { CalendarMenu } from './CalendarMenu'
import { getCalendarColorResolved } from './calendar-colors'

interface CalendarListProps {
    calendars: CalendarWithGroup[]
    visibleIds: Set<string>
    onToggle: (id: string) => void
    onColorChange: (calendarId: string, color: CalendarColorKey) => void
    onShowOnly: (calendarId: string) => void
}

function CalendarCheckbox({
    calendar,
    isChecked,
    onToggle,
    onColorChange,
    onShowOnly,
}: {
    calendar: CalendarWithGroup
    isChecked: boolean
    onToggle: (id: string) => void
    onColorChange: (calendarId: string, color: CalendarColorKey) => void
    onShowOnly: (calendarId: string) => void
}) {
    const theme = useTheme()
    const colors = getCalendarColorResolved(calendar.color)

    return (
        <View style={styles.calendarRow}>
            <Pressable style={styles.checkboxArea} onPress={() => onToggle(calendar.id)}>
                <View
                    style={[
                        styles.checkbox,
                        isChecked
                            ? { backgroundColor: colors.bg }
                            : { borderColor: colors.bg, borderWidth: 2 },
                    ]}
                >
                    {isChecked && <Check size={12} color={colors.text} />}
                </View>
                <Text style={[styles.calendarName, { color: theme.color.val }]} numberOfLines={1}>
                    {calendar.name}
                </Text>
            </Pressable>
            <CalendarMenu
                currentColor={calendar.color}
                onColorChange={color => onColorChange(calendar.id, color)}
                onShowOnly={() => onShowOnly(calendar.id)}
            />
        </View>
    )
}

function CalendarSection({
    title,
    calendars,
    visibleIds,
    onToggle,
    onColorChange,
    onShowOnly,
}: {
    title: string
    calendars: CalendarWithGroup[]
    visibleIds: Set<string>
    onToggle: (id: string) => void
    onColorChange: (calendarId: string, color: CalendarColorKey) => void
    onShowOnly: (calendarId: string) => void
}) {
    const [expanded, setExpanded] = useState(true)
    const theme = useTheme()
    const ChevronIcon = expanded ? ChevronDown : ChevronRight

    return (
        <View>
            <Pressable style={styles.sectionHeader} onPress={() => setExpanded(prev => !prev)}>
                <ChevronIcon size={14} color={theme.color8.val} />
                <Text style={[styles.sectionTitle, { color: theme.color8.val }]}>{title}</Text>
            </Pressable>
            {expanded &&
                calendars.map(cal => (
                    <CalendarCheckbox
                        key={cal.id}
                        calendar={cal}
                        isChecked={visibleIds.has(cal.id)}
                        onToggle={onToggle}
                        onColorChange={onColorChange}
                        onShowOnly={onShowOnly}
                    />
                ))}
        </View>
    )
}

export function CalendarList({
    calendars,
    visibleIds,
    onToggle,
    onColorChange,
    onShowOnly,
}: CalendarListProps) {
    const mine = calendars.filter(c => c.group === 'mine')
    const other = calendars.filter(c => c.group === 'other')

    return (
        <View style={styles.container}>
            <CalendarSection
                title="My calendars"
                calendars={mine}
                visibleIds={visibleIds}
                onToggle={onToggle}
                onColorChange={onColorChange}
                onShowOnly={onShowOnly}
            />
            <CalendarSection
                title="Other calendars"
                calendars={other}
                visibleIds={visibleIds}
                onToggle={onToggle}
                onColorChange={onColorChange}
                onShowOnly={onShowOnly}
            />
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        gap: 4,
    },
    sectionHeader: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 6,
        paddingHorizontal: 12,
        paddingVertical: 6,
    },
    sectionTitle: {
        fontSize: 12,
        fontWeight: '600',
    },
    calendarRow: {
        flexDirection: 'row',
        alignItems: 'center',
        paddingRight: 12,
        paddingVertical: 5,
    },
    checkboxArea: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 10,
        flex: 1,
        paddingLeft: 32,
    },
    checkbox: {
        width: 16,
        height: 16,
        borderRadius: 3,
        alignItems: 'center',
        justifyContent: 'center',
    },
    calendarName: {
        fontSize: 13,
        flex: 1,
    },
})
