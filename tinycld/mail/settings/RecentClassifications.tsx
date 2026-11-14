import { Text, View } from 'react-native'
import { useRecentClassifications } from '../classifier/use-recent-classifications'

type Row = ReturnType<typeof useRecentClassifications>[number]

function RecentRow({ row }: { row: Row }) {
    const detail = [row.label, row.confidence, row.stage, row.action].filter(Boolean).join(' · ')
    return (
        <View className="gap-0.5 py-2 border-b border-border" testID="mail-classifier-recent-row">
            <Text className="text-foreground text-sm" numberOfLines={1}>
                {row.subject}
            </Text>
            <Text className="text-muted-foreground text-xs">{detail}</Text>
        </View>
    )
}

function EmptyNote({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return <Text className="text-muted-foreground text-xs">Nothing has been classified yet.</Text>
}

export function RecentClassifications() {
    const rows = useRecentClassifications()
    const list = rows.map(row => <RecentRow key={row.id} row={row} />)
    return (
        <View className="gap-2" testID="mail-classifier-recent">
            <Text className="text-sm font-semibold text-foreground">Recent classifications</Text>
            <EmptyNote isVisible={rows.length === 0} />
            {list}
        </View>
    )
}
