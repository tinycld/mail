import { useParams, useRouter } from 'one'
import { useLiveQuery } from '@tanstack/react-db'
import { eq } from '@tanstack/db'
import { Pressable } from 'react-native'
import { ArrowLeft, Star } from 'lucide-react-native'
import { YStack, XStack, SizableText, Button, ScrollView, useTheme } from 'tamagui'
import { useForm, zodResolver } from '~/ui/form'
import { useStore } from '~/lib/pocketbase'
import { useMutation } from '~/lib/mutations'
import { handleMutationErrorsWithForm } from '~/lib/errors'
import { ContactAvatar } from '../components/ContactAvatar'
import { ContactForm } from '../components/ContactForm'
import { contactSchema } from '../components/contactSchema'

export default function ContactDetailScreen() {
    const router = useRouter()
    const theme = useTheme()
    const { id = '' } = useParams<{ id: string }>()
    const [contactsCollection] = useStore('contacts')

    const { data } = useLiveQuery(
        (query) =>
            query
                .from({ contacts: contactsCollection })
                .where(({ contacts }) => eq(contacts.id, id)),
        [id],
    )

    const contact = data?.[0] ?? null

    const {
        control,
        handleSubmit,
        setError,
        getValues,
        formState: { errors, isSubmitted },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(contactSchema),
        values: contact ? {
            first_name: contact.first_name ?? '',
            last_name: contact.last_name ?? '',
            email: contact.email ?? '',
            phone: contact.phone ?? '',
            company: contact.company ?? '',
            job_title: contact.job_title ?? '',
            notes: contact.notes ?? '',
            favorite: contact.favorite ?? false,
        } : undefined,
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

    const updateContact = useMutation({
        mutationFn: function* (formData: { first_name: string; last_name: string; email: string; phone: string; company: string; job_title: string; notes: string; favorite: boolean }) {
            yield contactsCollection.update(id, (draft) => {
                draft.first_name = formData.first_name.trim()
                draft.last_name = formData.last_name.trim()
                draft.email = formData.email
                draft.phone = formData.phone
                draft.company = formData.company.trim()
                draft.job_title = formData.job_title.trim()
                draft.notes = formData.notes
                draft.favorite = formData.favorite
            })
        },
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const toggleFavorite = useMutation({
        mutationFn: function* () {
            if (!contact) return
            yield contactsCollection.update(id, (draft) => {
                draft.favorite = !contact.favorite
            })
        },
    })

    const onSubmit = handleSubmit((formData) => updateContact.mutate(formData))

    if (!contact) {
        return (
            <YStack flex={1} padding="$5" backgroundColor="$background">
                <SizableText size="$4" color="$color8">
                    Contact not found
                </SizableText>
            </YStack>
        )
    }

    const displayName = [contact.first_name, contact.last_name].filter(Boolean).join(' ')

    return (
        <ScrollView contentContainerStyle={{ flexGrow: 1 }} backgroundColor="$background">
            <YStack flex={1} padding="$5">
                <XStack justifyContent="space-between" alignItems="center" marginBottom="$5">
                    <Pressable onPress={() => router.back()}>
                        <ArrowLeft size={24} color={theme.color.val} />
                    </Pressable>
                    <XStack gap="$3" alignItems="center">
                        <Pressable onPress={() => toggleFavorite.mutate()}>
                            <Star
                                size={24}
                                color={contact.favorite ? theme.yellow8.val : theme.color8.val}
                                fill={contact.favorite ? theme.yellow8.val : 'transparent'}
                            />
                        </Pressable>
                        <Button
                            theme="accent"
                            size="$3"
                            onPress={onSubmit}
                            disabled={updateContact.isPending}
                            opacity={updateContact.isPending ? 0.5 : 1}
                        >
                            <Button.Text fontWeight="600">
                                {updateContact.isPending ? 'Saving...' : 'Save'}
                            </Button.Text>
                        </Button>
                    </XStack>
                </XStack>

                <YStack alignItems="center" marginBottom="$5" gap="$2">
                    <ContactAvatar firstName={contact.first_name} lastName={contact.last_name} size={80} />
                    <SizableText size="$7" fontWeight="bold" color="$color">
                        {displayName}
                    </SizableText>
                    {contact.email ? (
                        <SizableText size="$3" color="$color8">
                            {contact.email}
                        </SizableText>
                    ) : null}
                </YStack>

                <ContactForm control={control} errors={errors} isSubmitted={isSubmitted} />
            </YStack>
        </ScrollView>
    )
}
