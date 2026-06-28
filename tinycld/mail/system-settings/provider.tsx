import { useLiveQuery } from '@tanstack/react-db'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import {
    type Control,
    NumberInput,
    SelectInput,
    TextInput,
    Toggle,
    useForm,
    z,
    zodResolver,
} from '@tinycld/core/ui/form'
import { newRecordId } from 'pbtsdb/core'
import { useWatch } from 'react-hook-form'
import { Text, View } from 'react-native'

// System-wide (deployment-default) mail provider config, contributed to the
// /admin Settings console via the manifest `systemSettings`. These values are the
// fallback the mail server uses when an org hasn't set its own provider override
// (see server/register.go providerForOrg). Stored in the shared system_settings
// collection under `mail.*` keys (the exact keys smtpConfigFromSystem reads); the
// console runs as a super-admin app user, so the writes are authorized.
//
// The form mirrors the per-org mail settings: a provider select swaps between
// Postmark (token credentials) and self-hosted SMTP (hostname + DKIM + an inbound
// mode that reveals the IMAP fetcher fields). Secret fields (tokens, IMAP
// password) are write-only — never seeded back into the form, and only persisted
// when re-entered.

const PROVIDER_OPTIONS = [
    { label: 'Postmark', value: 'postmark' },
    { label: 'Self-hosted SMTP', value: 'smtp' },
]

const INBOUND_MODE_OPTIONS = [
    { label: 'None (outbound only)', value: '' },
    { label: 'Built-in SMTP listener (we are the MX)', value: 'smtp' },
    { label: 'Poll IMAP mailbox', value: 'imap' },
]

const schema = z.object({
    provider: z.enum(['postmark', 'smtp']),
    // Secrets — blank means "leave the stored value unchanged" (write-only).
    postmark_server_token: z.string(),
    postmark_account_token: z.string(),
    smtp_public_hostname: z.string(),
    smtp_dkim_selector: z.string(),
    smtp_inbound_mode: z.enum(['', 'smtp', 'imap']),
    smtp_imap_host: z.string(),
    smtp_imap_port: z.number().int().min(0).max(65535),
    smtp_imap_username: z.string(),
    smtp_imap_password: z.string(), // secret
    smtp_imap_mailbox: z.string(),
    smtp_imap_poll_interval_seconds: z.number().int().min(0),
    smtp_imap_use_tls: z.boolean(),
})
type FormValues = z.infer<typeof schema>

// Which keys are secrets (write-only) vs always-persisted, and how each maps to a
// stored string under `mail.<key>`. Numbers/booleans are stored as the strings
// the Go reader parses (smtp_imap_use_tls is read as != "false").
const SECRET_KEYS = new Set([
    'postmark_server_token',
    'postmark_account_token',
    'smtp_imap_password',
])

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
    const get = (k: string) => byKey.get(`mail.${k}`)?.value ?? ''
    const num = (k: string, dflt: number) => {
        const n = Number.parseInt(get(k), 10)
        return Number.isNaN(n) ? dflt : n
    }

    const { control, handleSubmit } = useForm({
        resolver: zodResolver(schema),
        // Secrets are never seeded back (write-only); everything else reflects the
        // stored value so the form reactively shows current config.
        values: {
            provider: (get('provider') || 'postmark') as 'postmark' | 'smtp',
            postmark_server_token: '',
            postmark_account_token: '',
            smtp_public_hostname: get('smtp_public_hostname'),
            smtp_dkim_selector: get('smtp_dkim_selector'),
            smtp_inbound_mode: (get('smtp_inbound_mode') || '') as '' | 'smtp' | 'imap',
            smtp_imap_host: get('smtp_imap_host'),
            smtp_imap_port: num('smtp_imap_port', 0),
            smtp_imap_username: get('smtp_imap_username'),
            smtp_imap_password: '',
            smtp_imap_mailbox: get('smtp_imap_mailbox') || 'INBOX',
            smtp_imap_poll_interval_seconds: num('smtp_imap_poll_interval_seconds', 0),
            smtp_imap_use_tls: get('smtp_imap_use_tls') !== 'false',
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

    const persist = async (field: keyof FormValues, value: string) => {
        const isSecret = SECRET_KEYS.has(field)
        // Write-only secret: skip when blank so saving without re-entering leaves
        // the stored value untouched.
        if (isSecret && value.trim() === '') return
        await upsert.mutateAsync({ key: `mail.${field}`, value, isSecret })
    }

    const onSubmit = handleSubmit(async data => {
        await persist('provider', data.provider)
        if (data.provider === 'postmark') {
            await persist('postmark_server_token', data.postmark_server_token)
            await persist('postmark_account_token', data.postmark_account_token)
        } else {
            await persist('smtp_public_hostname', data.smtp_public_hostname)
            await persist('smtp_dkim_selector', data.smtp_dkim_selector)
            await persist('smtp_inbound_mode', data.smtp_inbound_mode)
            if (data.smtp_inbound_mode === 'imap') {
                await persist('smtp_imap_host', data.smtp_imap_host)
                await persist('smtp_imap_port', String(data.smtp_imap_port))
                await persist('smtp_imap_username', data.smtp_imap_username)
                await persist('smtp_imap_password', data.smtp_imap_password)
                await persist('smtp_imap_mailbox', data.smtp_imap_mailbox)
                await persist(
                    'smtp_imap_poll_interval_seconds',
                    String(data.smtp_imap_poll_interval_seconds)
                )
                await persist('smtp_imap_use_tls', String(data.smtp_imap_use_tls))
            }
        }
    })

    const serverTokenSet = Boolean(byKey.get('mail.postmark_server_token')?.value)
    const imapPasswordSet = Boolean(byKey.get('mail.smtp_imap_password')?.value)

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
            <ProviderFields
                control={control}
                serverTokenSet={serverTokenSet}
                imapPasswordSet={imapPasswordSet}
            />
            <View className="flex-row justify-end">
                <Button testID="mail-system-provider-save" onPress={onSubmit} size="sm">
                    <ButtonText>{upsert.isPending ? 'Saving…' : 'Save'}</ButtonText>
                </Button>
            </View>
        </View>
    )
}

// Swaps the credential/config inputs based on the selected provider, mirroring
// the per-org mail settings panel.
function ProviderFields({
    control,
    serverTokenSet,
    imapPasswordSet,
}: {
    control: Control<FormValues>
    serverTokenSet: boolean
    imapPasswordSet: boolean
}) {
    const provider = useWatch({ control, name: 'provider' })
    if (provider === 'smtp') {
        return <SmtpFields control={control} imapPasswordSet={imapPasswordSet} />
    }
    return <PostmarkFields control={control} serverTokenSet={serverTokenSet} />
}

function secretHint(set: boolean) {
    return set ? 'Configured. Enter a new value to replace it; leave blank to keep it.' : 'Not set.'
}

function PostmarkFields({
    control,
    serverTokenSet,
}: {
    control: Control<FormValues>
    serverTokenSet: boolean
}) {
    return (
        <>
            <TextInput
                control={control}
                name="postmark_server_token"
                label="Postmark server token"
                secureTextEntry
                autoCapitalize="none"
                hint={secretHint(serverTokenSet)}
            />
            <TextInput
                control={control}
                name="postmark_account_token"
                label="Postmark account token"
                secureTextEntry
                autoCapitalize="none"
                hint="Used for domain provisioning. Leave blank to keep the current value."
            />
        </>
    )
}

function SmtpFields({
    control,
    imapPasswordSet,
}: {
    control: Control<FormValues>
    imapPasswordSet: boolean
}) {
    const inboundMode = useWatch({ control, name: 'smtp_inbound_mode' })
    return (
        <>
            <TextInput
                control={control}
                name="smtp_public_hostname"
                label="Public hostname"
                placeholder="mx.example.com"
                autoCapitalize="none"
            />
            <TextInput
                control={control}
                name="smtp_dkim_selector"
                label="DKIM selector"
                placeholder="tinycld"
                autoCapitalize="none"
            />
            <SelectInput
                control={control}
                name="smtp_inbound_mode"
                label="Inbound mode"
                options={INBOUND_MODE_OPTIONS}
            />
            <ImapFieldsBlock
                isVisible={inboundMode === 'imap'}
                control={control}
                imapPasswordSet={imapPasswordSet}
            />
        </>
    )
}

function ImapFieldsBlock({
    isVisible,
    control,
    imapPasswordSet,
}: {
    isVisible: boolean
    control: Control<FormValues>
    imapPasswordSet: boolean
}) {
    if (!isVisible) return null
    return (
        <View className="gap-3 p-3 rounded-md border border-border">
            <Text className="text-foreground" style={{ fontWeight: '600' }}>
                IMAP fetcher
            </Text>
            <TextInput
                control={control}
                name="smtp_imap_host"
                label="IMAP host"
                placeholder="imap.example.com"
                autoCapitalize="none"
            />
            <NumberInput
                control={control}
                name="smtp_imap_port"
                label="IMAP port (0 = default by TLS)"
            />
            <TextInput
                control={control}
                name="smtp_imap_username"
                label="Username"
                autoCapitalize="none"
            />
            <TextInput
                control={control}
                name="smtp_imap_password"
                label="Password"
                secureTextEntry
                autoCapitalize="none"
                hint={secretHint(imapPasswordSet)}
            />
            <TextInput
                control={control}
                name="smtp_imap_mailbox"
                label="Mailbox"
                placeholder="INBOX"
                autoCapitalize="none"
            />
            <NumberInput
                control={control}
                name="smtp_imap_poll_interval_seconds"
                label="Poll interval (seconds)"
            />
            <Toggle control={control} name="smtp_imap_use_tls" label="Use TLS" />
        </View>
    )
}
