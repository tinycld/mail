import { TaxonomyEditor } from '@tinycld/core/components/classifier/TaxonomyEditor'
import { useClassifierAvailable } from '@tinycld/core/lib/classifier/use-classifier-available'
import { ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { FormErrorSummary, TextInput } from '@tinycld/core/ui/form'
import { ScrollView, Text, View } from 'react-native'
import { MAIL_TAXONOMY_PRESETS } from '../classifier/presets'
import { useArchiveThreshold } from '../classifier/use-archive-threshold'
import { ClassifierUnavailable } from './ClassifierUnavailable'

function ArchiveThresholdForm() {
    const { form, submit, isPending } = useArchiveThreshold()
    return (
        <View className="gap-2" testID="mail-classifier-archive-threshold">
            <FormErrorSummary
                errors={form.formState.errors}
                isEnabled={form.formState.isSubmitted}
            />
            <TextInput
                control={form.control}
                name="value"
                label="Archive confidence"
                hint="Eligible labels archive a message only at or above this confidence (0.5 to 1)."
                keyboardType="decimal-pad"
                testID="mail-classifier-archive-threshold-input"
            />
            <ServerActionButton
                size="sm"
                onPress={submit}
                isDisabled={isPending || !form.formState.isDirty}
                testID="mail-classifier-archive-threshold-save"
            >
                <ButtonText>Save</ButtonText>
            </ServerActionButton>
        </View>
    )
}

function OrgBody({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View className="gap-6">
            <TaxonomyEditor
                app="mail"
                scope="org"
                presets={MAIL_TAXONOMY_PRESETS}
                autoActionLabel="Archive eligible"
                isVisible
                testID="mail-classifier-org-editor"
            />
            <ArchiveThresholdForm />
        </View>
    )
}

export default function ClassificationOrg() {
    const { available, isKnown } = useClassifierAvailable()
    return (
        <ScrollView contentContainerClassName="gap-4 p-4">
            <Text className="text-lg font-semibold text-foreground">AI classification</Text>
            <Text className="text-muted-foreground text-sm">
                The labels every member starts with. Members can switch to their own labels.
            </Text>
            <ClassifierUnavailable isVisible={isKnown && !available} />
            <OrgBody isVisible={available} />
        </ScrollView>
    )
}
