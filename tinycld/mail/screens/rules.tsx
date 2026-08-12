import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { HelpIcon } from '@tinycld/core/components/help/HelpIcon'
import { RulesPanel } from '@tinycld/core/components/rules/RulesPanel'
import { Text, View } from 'react-native'
import { GestureHandlerRootView } from 'react-native-gesture-handler'

export default function MailRulesScreen() {
    return (
        <GestureHandlerRootView className="flex-1">
            <View className="flex-1 p-5 bg-background">
                <DocumentTitle pkg="Mail" title="Rules" />
                <View className="flex-row items-center gap-2 mb-4">
                    <Text className="text-2xl font-bold text-foreground">Mail rules</Text>
                    <HelpIcon topic="mail:rules" size={18} />
                </View>
                <RulesPanel scope="personal" pkgFilter="mail" canEdit />
            </View>
        </GestureHandlerRootView>
    )
}
