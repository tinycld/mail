import { useLiveQuery } from '@tanstack/react-db'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { SelectInput, TextInput, useForm } from '@tinycld/core/ui/form'
import { newRecordId } from 'pbtsdb/core'
import { Text, View } from 'react-native'

// System-wide (deployment-default) mail provider config, contributed to the
// /admin Settings console via the manifest `systemSettings`. These values are
// the fallback the mail server uses when an org hasn't set its own provider
// override (see server/register.go providerForOrg). Stored in the shared
// system_settings collection under `mail.*` keys; the console runs as a
// super-admin app user so the writes are authorized.

const PROVIDER_OPTIONS = [
    { label: 'Postmark', value: 'postmark' },
    { label: 'SMTP', value: 'smtp' },
]

const KEYS = {
    provider: 'mail.provider',
    serverToken: 'mail.postmark_server_token',
    accountToken: 'mail.postmark_account_token',
} as const

interface Row {
    id: string
    value: string
    is_secret: boolean
}

export default function MailSystemProvider() {
    const [systemSettings] = useStore('system_settings')
    const { data: rows = [] } = useLiveQuery(query => query.from({ s: systemSettings }))
    const byKey = new Map<string, Row>(
        rows.map(r => [r.key, { id: r.id, value: r.value, is_secret: r.is_secret }])
    )

    const provider = byKey.get(KEYS.provider)
    const serverToken = byKey.get(KEYS.serverToken)
    const accountToken = byKey.get(KEYS.accountToken)

    const {
        control,
        handleSubmit,
        formState: { isSubmitting },
    } = useForm({
        // The secret tokens are never seeded back into the form (write-only);
        // `values` reactively syncs the non-secret provider choice.
        values: {
            provider: provider?.value || 'postmark',
            serverToken: '',
            accountToken: '',
        },
        mode: 'onChange',
    })

    const upsert = useMutation({
        mutationFn: mutation(function* (input: { key: string; value: string; isSecret: boolean }) {
            const existing = byKey.get(input.key)
            if (existing) {
                yield systemSettings.update(existing.id, draft => {
                    draft.value = input.value
                })
            } else {
                yield systemSettings.insert({
                    id: newRecordId(),
                    key: input.key,
                    value: input.value,
                    is_secret: input.isSecret,
                } as never)
            }
        }),
    })

    const onSubmit = handleSubmit(async data => {
        await upsert.mutateAsync({ key: KEYS.provider, value: data.provider, isSecret: false })
        // Write-only secrets: only persist when re-entered, so saving without
        // retyping leaves the stored token untouched.
        if (data.serverToken.trim() !== '') {
            await upsert.mutateAsync({
                key: KEYS.serverToken,
                value: data.serverToken,
                isSecret: true,
            })
        }
        if (data.accountToken.trim() !== '') {
            await upsert.mutateAsync({
                key: KEYS.accountToken,
                value: data.accountToken,
                isSecret: true,
            })
        }
    })

    return (
        <View className="gap-4">
            <Text className="text-muted-foreground" style={{ fontSize: 13 }}>
                Default mail provider for this deployment. An organization can override these in its
                own mail settings; this is the fallback when it doesn't.
            </Text>
            <SelectInput
                control={control}
                name="provider"
                label="Provider"
                options={PROVIDER_OPTIONS}
            />
            <TextInput
                control={control}
                name="serverToken"
                label="Postmark server token"
                secureTextEntry
                autoCapitalize="none"
                placeholder={serverToken?.value ? '•••••••• (configured)' : 'Not set'}
                hint={
                    serverToken?.value
                        ? 'Configured. Enter a new value to replace it; leave blank to keep it.'
                        : 'Not set.'
                }
            />
            <TextInput
                control={control}
                name="accountToken"
                label="Postmark account token"
                secureTextEntry
                autoCapitalize="none"
                placeholder={accountToken?.value ? '•••••••• (configured)' : 'Not set'}
                hint={
                    accountToken?.value
                        ? 'Configured. Enter a new value to replace it; leave blank to keep it.'
                        : 'Not set.'
                }
            />
            <View className="flex-row justify-end">
                <Button
                    testID="mail-system-provider-save"
                    onPress={onSubmit}
                    size="sm"
                    isDisabled={isSubmitting}
                >
                    <ButtonText>{upsert.isPending ? 'Saving…' : 'Save'}</ButtonText>
                </Button>
            </View>
        </View>
    )
}
