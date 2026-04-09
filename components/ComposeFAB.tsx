import { Pencil } from 'lucide-react-native'
import { FAB } from '~/components/FAB'
import { composeEvents } from '../hooks/composeEvents'

interface ComposeFABProps {
    isVisible: boolean
}

export function ComposeFAB({ isVisible }: ComposeFABProps) {
    return (
        <FAB
            icon={Pencil}
            onPress={() => composeEvents.emit()}
            accessibilityLabel="Compose email"
            isVisible={isVisible}
        />
    )
}
