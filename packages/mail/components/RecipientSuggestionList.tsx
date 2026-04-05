import { Platform, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { ContactAvatar } from '../../contacts/components/ContactAvatar'

interface Suggestion {
    id: string
    first_name: string
    last_name: string
    email: string
}

interface RecipientSuggestionListProps {
    suggestions: Suggestion[]
    query: string
    onSelect: (contact: Suggestion) => void
}

const webShadow =
    Platform.OS === 'web'
        ? ({ boxShadow: '0 4px 16px rgba(0,0,0,0.18)' } as Record<string, unknown>)
        : {}

function HighlightText({
    text,
    query,
    style,
    highlightStyle,
}: {
    text: string
    query: string
    style: object
    highlightStyle: object
}) {
    if (!query) return <Text style={style}>{text}</Text>

    const lowerText = text.toLowerCase()
    const lowerQuery = query.toLowerCase()
    const index = lowerText.indexOf(lowerQuery)

    if (index === -1) return <Text style={style}>{text}</Text>

    return (
        <Text style={style}>
            {text.slice(0, index)}
            <Text style={highlightStyle}>{text.slice(index, index + query.length)}</Text>
            {text.slice(index + query.length)}
        </Text>
    )
}

export function RecipientSuggestionList({
    suggestions,
    query,
    onSelect,
}: RecipientSuggestionListProps) {
    const theme = useTheme()

    if (suggestions.length === 0) return null

    return (
        <View
            style={[
                styles.container,
                {
                    backgroundColor: theme.background.val,
                    borderColor: theme.borderColor.val,
                    ...webShadow,
                },
            ]}
        >
            <ScrollView style={styles.scroll} keyboardShouldPersistTaps="handled">
                {suggestions.map(contact => {
                    const fullName = [contact.first_name, contact.last_name]
                        .filter(Boolean)
                        .join(' ')

                    return (
                        <Pressable
                            key={contact.id}
                            style={({ pressed }) => [
                                styles.row,
                                pressed
                                    ? { backgroundColor: theme.backgroundHover.val }
                                    : undefined,
                            ]}
                            onPress={() => onSelect(contact)}
                        >
                            <ContactAvatar
                                firstName={contact.first_name}
                                lastName={contact.last_name}
                                size={32}
                            />
                            <View style={styles.textColumn}>
                                <HighlightText
                                    text={fullName}
                                    query={query}
                                    style={[styles.name, { color: theme.color.val }]}
                                    highlightStyle={styles.highlight}
                                />
                                <HighlightText
                                    text={contact.email ?? ''}
                                    query={query}
                                    style={[styles.email, { color: theme.color8.val }]}
                                    highlightStyle={styles.highlight}
                                />
                            </View>
                        </Pressable>
                    )
                })}
            </ScrollView>
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        position: 'absolute',
        top: '100%',
        left: 0,
        right: 0,
        zIndex: 2000,
        borderWidth: 1,
        borderRadius: 6,
        marginTop: 2,
    },
    scroll: {
        maxHeight: 250,
    },
    row: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 10,
        paddingHorizontal: 10,
        paddingVertical: 8,
    },
    textColumn: {
        flex: 1,
        gap: 1,
    },
    name: {
        fontSize: 13,
        fontWeight: '500',
    },
    email: {
        fontSize: 12,
    },
    highlight: {
        fontWeight: '700',
    },
})
