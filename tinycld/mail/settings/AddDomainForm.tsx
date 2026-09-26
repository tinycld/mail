import type { AddDomainResponse } from '@tinycld/app-generated/mail-api'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { FormErrorSummary, TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { Plus } from 'lucide-react-native'
import { Pressable, Text, View } from 'react-native'

const addDomainSchema = z.object({
    domain: z
        .string()
        .min(1, 'Domain is required')
        .regex(
            /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/,
            'Enter a valid domain'
        ),
})

// Shared by the Domains settings screen and the setup wizard's domain step,
// so both add a domain through the same form and endpoint.
// describeError lets a caller replace the server's message for an error it
// understands better in its own context; returning null keeps the default.
export function AddDomainForm({
    onAdded,
    describeError,
}: {
    onAdded?: (added: AddDomainResponse) => void
    describeError?: (error: unknown) => string | null
}) {
    const primaryFgColor = useThemeColor('primary-foreground')

    const {
        control,
        handleSubmit,
        setError,
        getValues,
        reset,
        formState: { errors, isSubmitted, isDirty },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(addDomainSchema),
        defaultValues: { domain: '' },
    })

    const handleFormError = handleMutationErrorsWithForm({ setError, getValues })
    const addMutation = useMutation({
        mutationFn: async (data: z.infer<typeof addDomainSchema>) =>
            pb.send<AddDomainResponse>('/api/mail/domains', {
                method: 'POST',
                body: { domain: data.domain },
            }),
        onSuccess: added => {
            reset()
            onAdded?.(added)
        },
        onError: error => {
            const message = describeError?.(error)
            if (message) {
                setError('domain', { type: 'manual', message })
                return
            }
            handleFormError(error)
        },
    })

    const onSubmit = handleSubmit(data => addMutation.mutate(data))
    const canSubmit = !addMutation.isPending && isDirty

    const addButton = (
        <Pressable
            onPress={onSubmit}
            disabled={!canSubmit}
            className={`flex-row items-center gap-1 px-4 rounded-lg py-2.5 bg-primary ${canSubmit ? 'opacity-100' : 'opacity-50'}`}
        >
            <Plus size={16} color={primaryFgColor} />
            <Text className="text-primary-foreground" style={{ fontWeight: '600' }}>
                {addMutation.isPending ? 'Adding...' : 'Add'}
            </Text>
        </Pressable>
    )

    return (
        <View className="gap-3">
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

            <TextInput
                control={control}
                name="domain"
                label="Add Domain"
                placeholder="example.com"
                wrapperProps={{ style: { marginBottom: 0 } }}
                addon={addButton}
            />
        </View>
    )
}
