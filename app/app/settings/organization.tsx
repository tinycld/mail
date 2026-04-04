import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { ArrowLeft } from 'lucide-react-native'
import { useRouter } from 'one'
import { Pressable } from 'react-native'
import { Button, ScrollView, SizableText, useTheme, XStack, YStack } from 'tamagui'
import { handleMutationErrorsWithForm } from '~/lib/errors'
import { useMutation } from '~/lib/mutations'
import { useStore } from '~/lib/pocketbase'
import { useCurrentRole } from '~/lib/use-current-role'
import { useOrgInfo } from '~/lib/use-org-info'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '~/ui/form'

const orgSchema = z.object({
    name: z.string().min(1, 'Organization name is required'),
})

export default function OrganizationSettings() {
    const router = useRouter()
    const theme = useTheme()
    const { isAdmin } = useCurrentRole()
    const { orgId } = useOrgInfo()
    const [orgsCollection] = useStore('orgs')

    const { data: orgs } = useLiveQuery(
        query => query.from({ orgs: orgsCollection }).where(({ orgs }) => eq(orgs.id, orgId)),
        [orgId]
    )
    const org = orgs?.[0]

    const {
        control,
        handleSubmit,
        setError,
        getValues,
        formState: { errors, isSubmitted, isDirty },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(orgSchema),
        values: { name: org?.name ?? '' },
    })

    const updateOrg = useMutation({
        mutationFn: function* (data: z.infer<typeof orgSchema>) {
            if (!orgId) throw new Error('No organization context')
            yield orgsCollection.update(orgId, draft => {
                draft.name = data.name.trim()
            })
        },
        onSuccess: () => router.back(),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const onSubmit = handleSubmit(data => updateOrg.mutate(data))
    const canSubmit = !updateOrg.isPending && isDirty && !!orgId

    if (!isAdmin) {
        return (
            <YStack flex={1} padding="$5" alignItems="center" justifyContent="center" backgroundColor="$background">
                <SizableText size="$5" color="$color8">
                    Only admins can manage organization settings.
                </SizableText>
            </YStack>
        )
    }

    return (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} backgroundColor="$background">
            <YStack flex={1} padding="$5" maxWidth={600}>
                <XStack justifyContent="space-between" alignItems="center" marginBottom="$5">
                    <XStack gap="$3" alignItems="center">
                        <Pressable onPress={() => router.back()}>
                            <ArrowLeft size={24} color={theme.color.val} />
                        </Pressable>
                        <SizableText size="$7" fontWeight="bold" color="$color">
                            Organization
                        </SizableText>
                    </XStack>
                    <Button
                        theme="accent"
                        size="$3"
                        onPress={onSubmit}
                        disabled={!canSubmit}
                        opacity={canSubmit ? 1 : 0.5}
                    >
                        <Button.Text fontWeight="600">
                            {updateOrg.isPending ? 'Saving...' : 'Save'}
                        </Button.Text>
                    </Button>
                </XStack>

                <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

                <YStack gap="$4">
                    <TextInput control={control} name="name" label="Organization Name" />

                    <YStack gap="$1">
                        <SizableText size="$3" color="$colorFocus">
                            Slug
                        </SizableText>
                        <SizableText size="$5" color="$color8">
                            {org?.slug ?? '—'}
                        </SizableText>
                    </YStack>
                </YStack>
            </YStack>
        </ScrollView>
    )
}
