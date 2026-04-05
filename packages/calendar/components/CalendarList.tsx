import { Check, ChevronDown, ChevronRight } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import type { CalendarWithGroup } from '../types'
import { getCalendarColorResolved } from './calendar-colors'

interface CalendarListProps {
    calendars: CalendarWithGroup[]
    visibleIds: Set<string>
    onToggle: (id: string) => void
}

function CalendarCheckbox({
    calendar,
    isChecked,
    onToggle,
}: {
    calendar: CalendarWithGroup
    isChecked: boolean
    onToggle: (id: string) => void
}) {
    const theme = useTheme()
    const colors = getCalendarColorResolved(calendar.color)

    return (
        <Pressable style={styles.calendarRow} onPress={() => onToggle(calendar.id)}>
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
            <Text style={[styles.calendarName, { color: theme.color.val }]}>{calendar.name}</Text>
        </Pressable>
    )
}

function CalendarSection({
    title,
    calendars,
    visibleIds,
    onToggle,
}: {
    title: string
    calendars: CalendarWithGroup[]
    visibleIds: Set<string>
    onToggle: (id: string) => void
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
                    />
                ))}
        </View>
    )
}

export function CalendarList({ calendars, visibleIds, onToggle }: CalendarListProps) {
    const mine = calendars.filter(c => c.group === 'mine')
    const other = calendars.filter(c => c.group === 'other')

    return (
        <View style={styles.container}>
            <CalendarSection
                title="My calendars"
                calendars={mine}
                visibleIds={visibleIds}
                onToggle={onToggle}
            />
            <CalendarSection
                title="Other calendars"
                calendars={other}
                visibleIds={visibleIds}
                onToggle={onToggle}
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
        gap: 10,
        paddingHorizontal: 12,
        paddingLeft: 32,
        paddingVertical: 5,
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
    },
})
