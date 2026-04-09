import { SizableText, YStack } from 'tamagui'

interface NotFoundStateProps {
    message: string
}

export function NotFoundState({ message }: NotFoundStateProps) {
    return (
        <YStack flex={1} alignItems="center" justifyContent="center" backgroundColor="$background">
            <SizableText size="$4" color="$color8">
                {message}
            </SizableText>
        </YStack>
    )
}
