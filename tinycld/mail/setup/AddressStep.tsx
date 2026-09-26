import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import type { SendEmailResponse } from '@tinycld/app-generated/mail-api'
import { useAuth } from '@tinycld/core/lib/auth'
import { errorToString } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Text, View } from 'react-native'
import { MailboxForm } from '~/tinycld/mail/settings/MailboxForm'
import {
    hasVerifiedDomain,
    isMailboxMember,
    testMessageRequest,
    verifiedDomainOptions,
} from '~/tinycld/mail/setup/setup-logic'

function useDomainRows() {
    const [domainsCollection] = useStore('mail_domains')
    const { data, isReady } = useLiveQuery(query =>
        query
            .from({ d: domainsCollection })
            .orderBy(({ d }) => d.created, 'asc')
            .select(({ d }) => ({ id: d.id, domain: d.domain, verified: d.verified }))
    )
    return { rows: data ?? [], isReady }
}

// A mailbox needs a verified domain, so the step waits for the domain step.
export function useIsStepVisible() {
    const { rows, isReady } = useDomainRows()
    return isReady ? hasVerifiedDomain(rows) : undefined
}

export function useIsStepDone() {
    const { user } = useAuth({ throwIfAnon: false })
    const [membersCollection] = useStore('mail_mailbox_members')
    const { data, isReady } = useMyLiveQuery((query, { userId }) =>
        query
            .from({ m: membersCollection })
            .where(({ m }) => eq(m.user, userId))
            .select(({ m }) => ({ user: m.user }))
    )
    return isReady ? isMailboxMember(data ?? [], user?.id) : undefined
}

// The newest mailbox the current user belongs to: the one this step created,
// or one made earlier if the person comes back to the step.
function useMyNewestMailbox() {
    const [membersCollection, mailboxesCollection, domainsCollection] = useStore(
        'mail_mailbox_members',
        'mail_mailboxes',
        'mail_domains'
    )
    const { data } = useMyLiveQuery((query, { userId }) =>
        query
            .from({ m: membersCollection })
            .innerJoin({ mb: mailboxesCollection }, ({ m, mb }) => eq(m.mailbox, mb.id))
            .innerJoin({ d: domainsCollection }, ({ mb, d }) => eq(mb.domain, d.id))
            .where(({ m }) => eq(m.user, userId))
            .orderBy(({ mb }) => mb.created, 'desc')
            .select(({ mb, d }) => ({ id: mb.id, address: mb.address, domain: d.domain }))
    )
    const newest = data?.[0]
    if (!newest) return null
    return { id: newest.id, email: `${newest.address}@${newest.domain}` }
}

function useTestMessage(mailboxId: string) {
    const { user } = useAuth()
    const { org } = useOrgInfo()
    const send = useMutation({
        mutationFn: async () =>
            pb.send<SendEmailResponse>('/api/mail/send', {
                method: 'POST',
                body: testMessageRequest({
                    mailboxId,
                    to: user.email,
                    workspaceName: org?.name ?? '',
                }),
            }),
    })
    return {
        to: user.email,
        send: () => send.mutate(),
        isPending: send.isPending,
        isSent: send.isSuccess,
        errorMessage: send.error ? errorToString(send.error) : null,
    }
}

function SentLabel({ isSent }: { isSent: boolean }) {
    if (!isSent) return null
    return <Text className="text-sm text-success">Sent</Text>
}

function ErrorText({ message }: { message: string | null }) {
    if (!message) return null
    return <Text className="text-sm text-danger">{message}</Text>
}

function CreatedMailbox({ mailbox }: { mailbox: { id: string; email: string } }) {
    const { to, send, isPending, isSent, errorMessage } = useTestMessage(mailbox.id)
    return (
        <View testID="setup-address-created" className="mb-4 gap-2">
            <Text className="text-sm text-foreground">Your address is</Text>
            <Text className="text-lg font-semibold text-foreground">{mailbox.email}</Text>
            <Text className="text-sm text-muted-foreground">
                To check that it can send, send a test message to {to}.
            </Text>
            <View className="flex-row items-center gap-3">
                <Button
                    variant="outline"
                    className="self-start"
                    onPress={send}
                    isDisabled={isPending}
                >
                    <ButtonText>{isPending ? 'Sending…' : 'Send a test message'}</ButtonText>
                </Button>
                <SentLabel isSent={isSent} />
            </View>
            <ErrorText message={errorMessage} />
        </View>
    )
}

function NewMailboxForm() {
    const { user } = useAuth()
    const { rows } = useDomainRows()
    return (
        <View className="mb-4">
            <MailboxForm
                mode="create"
                domainOptions={verifiedDomainOptions(rows)}
                userId={user.id}
                onDone={noop}
            />
        </View>
    )
}

// The new mailbox appears through the live query, so there is nothing to do on
// completion.
function noop() {}

function AddressBody({ mailbox }: { mailbox: { id: string; email: string } | null }) {
    if (!mailbox) return <NewMailboxForm />
    return <CreatedMailbox mailbox={mailbox} />
}

export default function AddressStep({ next }: SetupStepProps) {
    const mailbox = useMyNewestMailbox()
    return (
        <View className="max-w-[440px] gap-1">
            <Text className="text-2xl font-bold text-foreground">Your email address</Text>
            <Text className="mb-3 text-sm text-muted-foreground">
                Make the address you will send and receive email from.
            </Text>
            <AddressBody mailbox={mailbox} />
            <Button className="self-start" onPress={next}>
                <ButtonText>Continue</ButtonText>
            </Button>
        </View>
    )
}
