import type { AddDomainResponse } from '@tinycld/app-generated/mail-api'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb } from '@tinycld/core/lib/pocketbase'
import { ButtonIcon, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { TextInput, useForm, z, zodResolver } from '@tinycld/core/ui/form'
import { Plus } from 'lucide-react-native'
import { View } from 'react-native'
import { addDomainErrorMessage } from './add-domain-error'

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
    const {
        control,
        handleSubmit,
        setError,
        reset,
        formState: { isDirty },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(addDomainSchema),
        defaultValues: { domain: '' },
    })

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
            setError('domain', {
                type: 'manual',
                message: addDomainErrorMessage(error, describeError),
            })
        },
    })

    const onSubmit = handleSubmit(data => addMutation.mutate(data))
    const canSubmit = !addMutation.isPending && isDirty

    const addButton = (
        <ServerActionButton onPress={onSubmit} isDisabled={!canSubmit}>
            <ButtonIcon as={Plus} />
            <ButtonText>{addMutation.isPending ? 'Adding...' : 'Add'}</ButtonText>
        </ServerActionButton>
    )

    // No FormErrorSummary: every error lands on the one field, so a summary
    // would only repeat the message under the input.
    return (
        <View className="gap-3">
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
