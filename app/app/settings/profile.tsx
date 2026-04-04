import { ArrowLeft } from 'lucide-react-native'
import { useRouter } from 'one'
import { Pressable } from 'react-native'
import { Button, ScrollView, SizableText, useTheme, XStack, YStack } from 'tamagui'
import { useAuth } from '~/lib/auth'
import { handleMutationErrorsWithForm } from '~/lib/errors'
import { useMutation } from '~/lib/mutations'
import { useStore } from '~/lib/pocketbase'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '~/ui/form'

const profileSchema = z.object({
    name: z.string().min(1, 'Name is required'),
    email: z.string().email('Valid email is required'),
})

export default function ProfileSettings() {
    const router = useRouter()
    const theme = useTheme()
    const { user } = useAuth()
    const [usersCollection] = useStore('users')

    const {
        control,
        handleSubmit,
        setError,
        getValues,
        formState: { errors, isSubmitted, isDirty },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(profileSchema),
        values: { name: user.name, email: user.email },
    })

    const updateProfile = useMutation({
        mutationFn: function* (data: z.infer<typeof profileSchema>) {
            yield usersCollection.update(user.id, draft => {
                draft.name = data.name.trim()
                draft.email = data.email.trim()
            })
        },
        onSuccess: () => router.back(),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const onSubmit = handleSubmit(data => updateProfile.mutate(data))
    const canSubmit = !updateProfile.isPending && isDirty

    return (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} backgroundColor="$background">
            <YStack flex={1} padding="$5" maxWidth={600}>
                <XStack justifyContent="space-between" alignItems="center" marginBottom="$5">
                    <XStack gap="$3" alignItems="center">
                        <Pressable onPress={() => router.back()}>
                            <ArrowLeft size={24} color={theme.color.val} />
                        </Pressable>
                        <SizableText size="$7" fontWeight="bold" color="$color">
                            Profile
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
                            {updateProfile.isPending ? 'Saving...' : 'Save'}
                        </Button.Text>
                    </Button>
                </XStack>

                <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

                <YStack gap="$4">
                    <TextInput control={control} name="name" label="Name" />
                    <TextInput control={control} name="email" label="Email" />
                </YStack>
            </YStack>
        </ScrollView>
    )
}
