import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { Sparkles } from 'lucide-react-native'
import { Text, View } from 'react-native'
import type { useThreadClassification } from '../hooks/useThreadClassification'

type Classification = NonNullable<ReturnType<typeof useThreadClassification>>

interface AiLabelChipProps {
    classification: Classification | undefined
    isVisible: boolean
}

export function AiLabelChip({ classification, isVisible }: AiLabelChipProps) {
    const iconColor = useThemeColor('muted-foreground')
    if (!isVisible || !classification) return null
    const hint =
        classification.stage === 'full' ? 'Classified from full text' : 'Classified from metadata'
    return (
        <View
            className="flex-row items-center gap-1 rounded-full bg-muted px-2 py-0.5"
            accessibilityHint={hint}
            testID="mail-ai-chip"
        >
            <Sparkles size={12} color={iconColor} />
            <Text className="text-xs text-muted-foreground">
                {`AI · ${classification.label} · ${classification.confidencePct}%`}
            </Text>
        </View>
    )
}
