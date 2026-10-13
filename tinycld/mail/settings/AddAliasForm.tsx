import { eq, inArray } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { handleMutationErrorsWithForm } from '@tinycld/core/lib/errors'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { ButtonIcon, ButtonText, ServerActionButton } from '@tinycld/core/ui/button'
import { FormErrorSummary, TextInput, useForm, zodResolver } from '@tinycld/core/ui/form'
import { Plus } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb/core'
import { useMemo } from 'react'
import { Text, View } from 'react-native'
import { type AliasFormValues, aliasSchemaFor } from './aliasSchema'

interface Props {
    mailboxId: string
    mailboxDomainId: string
    domainName: string
}

export function AddAliasForm({ mailboxId, mailboxDomainId, domainName }: Props) {
    const [aliasesCollection, mailboxesCollection] = useStore(
        'mail_mailbox_aliases',
        'mail_mailboxes'
    )

    const { data: mailboxesInDomain } = useLiveQuery({
        query: query =>
            query
                .from({ mail_mailboxes: mailboxesCollection })
                .where(({ mail_mailboxes }) => eq(mail_mailboxes.domain, mailboxDomainId)),
    })
    const mailboxIdsInDomain = (mailboxesInDomain ?? []).map(m => m.id)

    // The org-wide alias list rule means an unscoped read here pulls every
    // alias in the deployment to validate one new one. Scope to the mailboxes
    // already fetched above — the aliases this form can actually collide
    // with — instead of the whole mail_mailbox_aliases table.
    const { data: aliasesInDomain } = useLiveQuery({
        query: query =>
            mailboxIdsInDomain.length === 0
                ? undefined
                : query
                      .from({ mail_mailbox_aliases: aliasesCollection })
                      .where(({ mail_mailbox_aliases }) =>
                          inArray(mail_mailbox_aliases.mailbox, mailboxIdsInDomain)
                      ),
    })

    const schema = useMemo(
        () =>
            aliasSchemaFor({
                mailboxAddresses: new Set((mailboxesInDomain ?? []).map(m => m.address)),
                aliasAddresses: new Set((aliasesInDomain ?? []).map(a => a.address)),
            }),
        [mailboxesInDomain, aliasesInDomain]
    )

    const {
        control,
        handleSubmit,
        reset,
        setError,
        getValues,
        formState: { errors, isSubmitted, isValid },
    } = useForm({
        mode: 'onChange',
        resolver: zodResolver(schema),
        defaultValues: { address: '' },
    })

    const create = useMutation({
        mutationFn: mutation(function* (data: AliasFormValues) {
            yield aliasesCollection.insert({
                id: newRecordId(),
                mailbox: mailboxId,
                address: data.address,
                created: '',
                updated: '',
            })
        }),
        onSuccess: () => reset(),
        onError: handleMutationErrorsWithForm({ setError, getValues }),
    })

    const onSubmit = handleSubmit(d => create.mutate(d))

    return (
        <View className="gap-2">
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />
            <View
                className="flex-row gap-2 items-center rounded-lg px-2 border border-border"
                style={{ borderStyle: 'dashed' }}
            >
                <View className="flex-1">
                    <TextInput control={control} name="address" placeholder="new-alias" />
                </View>
                <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                    @{domainName}
                </Text>
                <ServerActionButton
                    size="sm"
                    onPress={onSubmit}
                    isDisabled={!isValid || create.isPending}
                >
                    <ButtonIcon as={Plus} />
                    <ButtonText>Add</ButtonText>
                </ServerActionButton>
            </View>
        </View>
    )
}
