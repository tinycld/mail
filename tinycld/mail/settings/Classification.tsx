import { TaxonomyEditor } from '@tinycld/core/components/classifier/TaxonomyEditor'
import { useClassifierAvailable } from '@tinycld/core/lib/classifier/use-classifier-available'
import { useTaxonomy } from '@tinycld/core/lib/classifier/use-taxonomy'
import { SelectInput, useForm } from '@tinycld/core/ui/form'
import { ScrollView, Text, View } from 'react-native'
import { MAIL_TAXONOMY_PRESETS } from '../classifier/presets'
import {
    type ClassifierMode,
    type TaxonomyChoice,
    useClassifierPrefs,
} from '../classifier/use-classifier-prefs'
import { ClassifierUnavailable } from './ClassifierUnavailable'
import { RecentClassifications } from './RecentClassifications'

const MODE_OPTIONS = [
    { value: 'off', label: 'Off' },
    { value: 'preview', label: 'Preview only' },
    { value: 'labels', label: 'Labels' },
    { value: 'labels_archive', label: 'Labels and archive' },
]

const TAXONOMY_OPTIONS = [
    { value: 'org', label: 'Organization default' },
    { value: 'personal', label: 'My own labels' },
]

function useClassificationSettings() {
    const prefs = useClassifierPrefs()
    const { rows: orgRows } = useTaxonomy('mail', 'org')
    const { control } = useForm({
        values: { mode: prefs.mode, taxonomy: prefs.taxonomy },
    })
    // An empty org taxonomy would make the copy preset wipe the editor.
    const orgPresets =
        orgRows.length === 0
            ? []
            : [
                  {
                      id: 'org',
                      label: 'Copy of organization labels',
                      rows: orgRows.map(({ name, assignWhen, autoAction }) => ({
                          name,
                          assignWhen,
                          autoAction,
                      })),
                  },
              ]
    return {
        control,
        taxonomy: prefs.taxonomy,
        presets: [...orgPresets, ...MAIL_TAXONOMY_PRESETS],
        onModeChange: (value: string) => prefs.setMode(value as ClassifierMode),
        onTaxonomyChange: (value: string) => prefs.setTaxonomy(value as TaxonomyChoice),
    }
}

function AccountBody({ isVisible }: { isVisible: boolean }) {
    const settings = useClassificationSettings()
    if (!isVisible) return null
    return (
        <View className="gap-4">
            <SelectInput
                control={settings.control}
                name="mode"
                label="Classify incoming mail"
                options={MODE_OPTIONS}
                onValueChange={settings.onModeChange}
                testID="mail-classifier-mode"
            />
            <SelectInput
                control={settings.control}
                name="taxonomy"
                label="Labels to use"
                options={TAXONOMY_OPTIONS}
                onValueChange={settings.onTaxonomyChange}
                testID="mail-classifier-taxonomy"
            />
            <TaxonomyEditor
                app="mail"
                scope="personal"
                presets={settings.presets}
                autoActionLabel="Archive eligible"
                isVisible={settings.taxonomy === 'personal'}
                testID="mail-classifier-personal-editor"
            />
            <RecentClassifications />
        </View>
    )
}

export default function Classification() {
    const { available, isKnown } = useClassifierAvailable()
    return (
        <ScrollView contentContainerClassName="gap-4 p-4">
            <Text className="text-lg font-semibold text-foreground">AI classification</Text>
            <ClassifierUnavailable isVisible={isKnown && !available} />
            <AccountBody isVisible={available} />
        </ScrollView>
    )
}
