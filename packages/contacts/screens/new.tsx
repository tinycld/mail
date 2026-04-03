import { useRouter } from 'one'
import { Pressable } from 'react-native'
import { ArrowLeft } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb'
import { YStack, XStack, SizableText, Button, ScrollView, useTheme } from 'tamagui'
import { useForm, zodResolver, z } from '~/ui/form'
import { useStore } from '~/lib/pocketbase'
import { useOrgInfo } from '~/lib/use-org-info'
import { useMutation } from '~/lib/mutations'
import { handleMutationErrorsWithForm } from '~/lib/errors'
import { useCurrentUserOrg } from '../hooks/useCurrentUserOrg'
import { ContactForm } from '../components/ContactForm'
import { contactSchema } from '../components/contactSchema'

export default function NewContactScreen() {
    const router = useRouter()
    const theme = useTheme()
    const { orgSlug } = useOrgInfo()
    const userOrg = useCurrentUserOrg(orgSlug)
    const [contactsCollection] = useStore('contacts')

    const {
        control,
        handleSubmit,
        setError,
        getValues,
        formState: { errors, isSubmitted },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(contactSchema),
        defaultValues: {
            first_name: '',
            last_name: '',
            email: '',
            phone: '',
            company: '',
            job_title: '',
            notes: '',
            favorite: false,
        },
    })

    const createContact = useMutation({
        mutationFn: function* (data: z.infer<typeof contactSchema>) {
            if (!userOrg) throw new Error('No organization context')
            yield contactsCollection.insert({
                id: newRecordId(),
                first_name: data.first_name.trim(),
                last_name: data.last_name.trim(),
                email: data.email,
                phone: data.phone,
                company: data.company.trim(),
                job_title: data.job_title.trim(),
                notes: data.notes,
                favorite: data.favorite,
                owner: userOrg.id,
            })
        },
        onSuccess: () => router.back(),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const onSubmit = handleSubmit((data) => createContact.mutate(data))
    const canSubmit = !createContact.isPending && !!userOrg

    return (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} backgroundColor="$background">
            <YStack flex={1} padding="$5">
                <XStack justifyContent="space-between" alignItems="center" marginBottom="$5">
                    <XStack gap="$3" alignItems="center">
                        <Pressable onPress={() => router.back()}>
                            <ArrowLeft size={24} color={theme.color.val} />
                        </Pressable>
                        <SizableText size="$7" fontWeight="bold" color="$color">
                            Create Contact
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
                            {createContact.isPending ? 'Creating...' : 'Create'}
                        </Button.Text>
                    </Button>
                </XStack>

                <ContactForm control={control} errors={errors} isSubmitted={isSubmitted} />
            </YStack>
        </ScrollView>
    )
}
