import { TaxonomyEditor } from '@tinycld/core/components/classifier/TaxonomyEditor'
import { HelpIcon } from '@tinycld/core/components/help/HelpIcon'
import { useClassifierAvailable } from '@tinycld/core/lib/classifier/use-classifier-available'
import { ScrollView, Text, View } from 'react-native'
import { MAIL_TAXONOMY_PRESETS } from '../classifier/presets'
import { ClassifierUnavailable } from './ClassifierUnavailable'

function OrgBody({ isVisible }: { isVisible: boolean }) {
    return (
        <TaxonomyEditor
            app="mail"
            scope="org"
            presets={MAIL_TAXONOMY_PRESETS}
            autoActionLabel="Archive eligible"
            isVisible={isVisible}
            testID="mail-classifier-org-editor"
        />
    )
}

export default function ClassificationOrg() {
    const { available, isKnown } = useClassifierAvailable()
    return (
        <ScrollView contentContainerClassName="gap-4 p-4">
            <View className="flex-row items-center gap-1.5">
                <Text className="text-lg font-semibold text-foreground">AI classification</Text>
                <HelpIcon topic="mail:ai-classification" />
            </View>
            <Text className="text-muted-foreground text-sm">
                The labels every member starts with. Members can switch to their own labels.
            </Text>
            <ClassifierUnavailable isVisible={isKnown && !available} />
            <OrgBody isVisible={available} />
        </ScrollView>
    )
}
