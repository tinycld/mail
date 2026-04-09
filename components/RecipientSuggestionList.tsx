import { Platform, ScrollView, StyleSheet } from 'react-native'
import { ListItem, SizableText, YStack } from 'tamagui'
import { NameAvatar as ContactAvatar } from '~/components/NameAvatar'

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

function HighlightText({ text, query, bold }: { text: string; query: string; bold?: boolean }) {
    if (!query) {
        return (
            <SizableText size="$3" fontWeight={bold ? '500' : undefined}>
                {text}
            </SizableText>
        )
    }

    const lowerText = text.toLowerCase()
    const lowerQuery = query.toLowerCase()
    const index = lowerText.indexOf(lowerQuery)

    if (index === -1) {
        return (
            <SizableText size="$3" fontWeight={bold ? '500' : undefined}>
                {text}
            </SizableText>
        )
    }

    return (
        <SizableText size="$3" fontWeight={bold ? '500' : undefined}>
            {text.slice(0, index)}
            <SizableText size="$3" fontWeight="700">
                {text.slice(index, index + query.length)}
            </SizableText>
            {text.slice(index + query.length)}
        </SizableText>
    )
}

export function RecipientSuggestionList({
    suggestions,
    query,
    onSelect,
}: RecipientSuggestionListProps) {
    if (suggestions.length === 0) return null

    return (
        <YStack
            position="absolute"
            top="100%"
            left={0}
            right={0}
            zIndex={2000}
            marginTop={2}
            borderWidth={1}
            borderRadius="$2"
            borderColor="$borderColor"
            backgroundColor="$background"
            overflow="hidden"
            {...(webShadow as object)}
        >
            <ScrollView style={styles.scroll} keyboardShouldPersistTaps="handled">
                {suggestions.map(contact => {
                    const fullName = [contact.first_name, contact.last_name]
                        .filter(Boolean)
                        .join(' ')

                    return (
                        <ListItem
                            key={contact.id}
                            size="$3"
                            icon={
                                <ContactAvatar
                                    firstName={contact.first_name}
                                    lastName={contact.last_name}
                                    size={32}
                                />
                            }
                            title={<HighlightText text={fullName} query={query} bold />}
                            subTitle={<HighlightText text={contact.email ?? ''} query={query} />}
                            onPress={() => onSelect(contact)}
                            hoverStyle={{ backgroundColor: '$backgroundHover' }}
                            pressStyle={{ backgroundColor: '$backgroundPress' }}
                            cursor="pointer"
                            gap="$2"
                        />
                    )
                })}
            </ScrollView>
        </YStack>
    )
}

const styles = StyleSheet.create({
    scroll: {
        maxHeight: 250,
    },
})
