import { useThemeColor } from 'heroui-native'
import { type Control, type Path, useController } from 'react-hook-form'
import { Pressable, Text, View } from 'react-native'
import { NameAvatar as ContactAvatar } from '~/components/NameAvatar'
import { PlainInput } from '~/ui/PlainInput'
import type { ComposeFormData } from '../hooks/composeSchema'
import { useRecipientSuggestions } from '../hooks/useRecipientSuggestions'
import { RecipientSuggestionList } from './RecipientSuggestionList'

interface RecipientFieldProps {
    control: Control<ComposeFormData>
    name: Path<ComposeFormData>
    placeholder?: string
}

function RecipientChip({
    name,
    email,
    onRemove,
}: {
    name: string
    email: string
    onRemove: () => void
}) {
    const [foregroundColor, mutedColor, surfaceColor, borderColor] = useThemeColor([
        'foreground',
        'muted',
        'surface-secondary',
        'border',
    ])
    const displayName = name || email
    const firstName = name.split(' ')[0] || email.split('@')[0]
    const lastName = name.split(' ').slice(1).join(' ')

    return (
        <View
            style={{
                flexDirection: 'row',
                alignItems: 'center',
                gap: 4,
                paddingLeft: 2,
                paddingRight: 6,
                paddingVertical: 2,
                borderRadius: 12,
                borderWidth: 1,
                backgroundColor: surfaceColor,
                borderColor,
            }}
        >
            <ContactAvatar firstName={firstName} lastName={lastName} size={18} />
            <Text style={{ fontSize: 12, maxWidth: 140, color: foregroundColor }} numberOfLines={1}>
                {displayName}
            </Text>
            <Pressable onPress={onRemove} hitSlop={4}>
                <Text
                    style={{ fontSize: 15, lineHeight: 16, fontWeight: '600', color: mutedColor }}
                >
                    x
                </Text>
            </Pressable>
        </View>
    )
}

export function RecipientField({ control, name, placeholder }: RecipientFieldProps) {
    const [foregroundColor, placeholderColor] = useThemeColor(['foreground', 'field-placeholder'])
    const { field } = useController({ control, name })

    const { committedRecipients, activeQuery, committedRaw, suggestions, showSuggestions } =
        useRecipientSuggestions(field.value)

    const handleSelect = (contact: { first_name: string; last_name: string; email: string }) => {
        const fullName = [contact.first_name, contact.last_name].filter(Boolean).join(' ')
        const formatted = `${fullName} <${contact.email}>`
        field.onChange(`${committedRaw}${formatted}, `)
    }

    const handleRemoveChip = (index: number) => {
        const updated = committedRecipients.filter((_, i) => i !== index)
        const joined = updated.map(r => (r.name ? `${r.name} <${r.email}>` : r.email)).join(', ')
        const newRaw = updated.length > 0 ? `${joined}, ` : ''
        field.onChange(`${newRaw}${activeQuery}`)
    }

    const handleChangeText = (text: string) => {
        field.onChange(committedRaw + text)
    }

    return (
        <View style={{ position: 'relative', flex: 1 }}>
            <View
                style={{
                    flexDirection: 'row',
                    flexWrap: 'wrap',
                    alignItems: 'center',
                    gap: 4,
                    minHeight: 28,
                }}
            >
                {committedRecipients.map((r, i) => {
                    return (
                        <RecipientChip
                            key={r.email}
                            name={r.name}
                            email={r.email}
                            onRemove={() => handleRemoveChip(i)}
                        />
                    )
                })}
                <PlainInput
                    style={{
                        flex: 1,
                        fontSize: 13,
                        minWidth: 80,
                        paddingHorizontal: 4,
                        color: foregroundColor,
                    }}
                    placeholderTextColor={placeholderColor}
                    value={activeQuery}
                    onChangeText={handleChangeText}
                    onBlur={field.onBlur}
                    placeholder={committedRecipients.length === 0 ? placeholder : undefined}
                />
            </View>
            {showSuggestions ? (
                <RecipientSuggestionList
                    suggestions={suggestions}
                    query={activeQuery}
                    onSelect={handleSelect}
                />
            ) : null}
        </View>
    )
}
