import { useMutation } from '@tinycld/core/lib/mutations'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import * as Clipboard from 'expo-clipboard'
import { Check, Copy } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable } from 'react-native'

/** An icon button that copies value; it shows a check for a moment after. */
export function CopyButton({ value, label }: { value: string; label: string }) {
    const mutedColor = useThemeColor('muted-foreground')
    const successColor = useThemeColor('success')
    const [copied, setCopied] = useState(false)
    const copy = useMutation({
        mutationFn: async () => {
            await Clipboard.setStringAsync(value)
        },
        onSuccess: () => {
            setCopied(true)
            setTimeout(() => setCopied(false), 1500)
        },
    })
    const Icon = copied ? Check : Copy
    return (
        <Pressable onPress={() => copy.mutate()} accessibilityLabel={label}>
            <Icon size={14} color={copied ? successColor : mutedColor} />
        </Pressable>
    )
}
