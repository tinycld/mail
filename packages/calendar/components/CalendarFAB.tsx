import { Plus } from 'lucide-react-native'
import { useRouter } from 'one'
import { FAB } from '~/components/FAB'
import { useOrgHref } from '~/lib/org-routes'

interface CalendarFABProps {
    isVisible: boolean
}

export function CalendarFAB({ isVisible }: CalendarFABProps) {
    const router = useRouter()
    const orgHref = useOrgHref()

    return (
        <FAB
            icon={Plus}
            onPress={() => router.push(orgHref('calendar/[id]', { id: 'new' }))}
            accessibilityLabel="Create event"
            isVisible={isVisible}
            iconSize={24}
        />
    )
}
