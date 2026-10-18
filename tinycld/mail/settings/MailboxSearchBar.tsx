import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Search } from 'lucide-react-native'
import { Platform, TextInput, View } from 'react-native'

interface Props {
    query: string
    onQueryChange: (next: string) => void
}

export function MailboxSearchBar({ query, onQueryChange }: Props) {
    const borderColor = useThemeColor('border')
    const mutedColor = useThemeColor('muted-foreground')
    const fgColor = useThemeColor('foreground')
    const bgColor = useThemeColor('background')

    return (
        <View className="flex-row gap-2 items-center flex-wrap">
            <View
                className="flex-row gap-2 items-center flex-1 rounded-lg px-3"
                style={{
                    borderWidth: 1,
                    borderColor,
                    paddingVertical: 8,
                    backgroundColor: bgColor,
                    minWidth: 200,
                }}
            >
                <Search size={14} color={mutedColor} />
                <TextInput
                    value={query}
                    onChangeText={onQueryChange}
                    placeholder="Search address, display name, or member…"
                    placeholderTextColor={mutedColor}
                    style={{
                        flex: 1,
                        color: fgColor,
                        fontSize: 13,
                        padding: 0,
                        ...(Platform.OS === 'web'
                            ? ({ outlineStyle: 'none' } as Record<string, unknown>)
                            : {}),
                    }}
                    autoCorrect={false}
                    autoCapitalize="none"
                />
            </View>
        </View>
    )
}
