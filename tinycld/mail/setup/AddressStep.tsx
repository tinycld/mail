import { eq, gt, inArray } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import type { SendEmailResponse } from '@tinycld/app-generated/mail-api'
import { HelpIcon } from '@tinycld/core/components/help/HelpIcon'
import { SetupContinueButton } from '@tinycld/core/components/setup/wizard/SetupContinueButton'
import { useAuth } from '@tinycld/core/lib/auth'
import { errorToString } from '@tinycld/core/lib/errors'
import { useMutation } from '@tinycld/core/lib/mutations'
import { pb, useStore } from '@tinycld/core/lib/pocketbase'
import type { SetupStepProps } from '@tinycld/core/lib/setup/types'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { useOrgInfo } from '@tinycld/core/lib/use-org-info'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { useRouter } from 'expo-router'
import { Text, View } from 'react-native'
import { MailboxForm } from '../settings/MailboxForm'
import { defaultAddressFor } from '../settings/mailbox-records'
import {
    EMAIL_DOMAIN_STEP_HREF,
    hasVerifiedDomain,
    type TestMessageState,
    testMessageLabel,
    testMessageRequest,
    testMessageState,
    verifiedDomainOptions,
} from './setup-logic'

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

// Done only when a message has arrived. The mail_messages list rule already
// limits rows to mailboxes the user is a member of, so a single-collection
// query answers "delivered in one of my mailboxes" with one limit-1 request.
// A join through members and threads would load every thread of every mailbox
// the user can see, and this hook runs on each wizard render.
export function useIsStepDone() {
    const [messagesCollection] = useStore('mail_messages')
    const { data, isReady } = useLiveQuery(query =>
        query
            .from({ msg: messagesCollection })
            .where(({ msg }) => gt(msg.delivered_at, ''))
            .select(({ msg }) => ({ id: msg.id }))
            .findOne()
    )
    return isReady ? data !== undefined : undefined
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
        errorMessage: send.error ? errorToString(send.error) : null,
    }
}

const OUTBOUND_STATUSES = ['sending', 'sent', 'bounced', 'spam_complaint']

// The newest message the user sent from this mailbox's address. It is read
// from the store, not from the send response, so the status is correct after
// a reload.
//
// Filtered by sent_by AND sender_email — still one collection, no thread
// join — so this stays a single server-side filter (mailbox-scoped, not just
// user-scoped): an owner with other mailboxes, or who has sent other mail,
// must see this mailbox's test message, not merely their own newest one.
// sender_email is stored verbatim as `${mailbox.address}@${domain.domain}`
// (endpoints_send.go: senderEmail := fmt.Sprintf("%s@%s", senderAddress,
// domain), no case normalization) — the same fields senderEmail here is built
// from (useMyNewestMailbox), so the comparison is exact-case.
function useTestMessageState(senderEmail: string) {
    const [messagesCollection] = useStore('mail_messages')
    const { data } = useMyLiveQuery((query, { userId }) =>
        query
            .from({ msg: messagesCollection })
            .where(({ msg }) => eq(msg.sent_by, userId))
            .where(({ msg }) => eq(msg.sender_email, senderEmail))
            .where(({ msg }) => inArray(msg.delivery_status, OUTBOUND_STATUSES))
            .orderBy(({ msg }) => msg.created, 'desc')
            .select(({ msg }) => ({
                delivery_status: msg.delivery_status,
                delivered_at: msg.delivered_at,
                bounce_class: msg.bounce_class,
                bounce_reason: msg.bounce_reason,
            }))
            .findOne()
    )
    return testMessageState(data)
}

function BouncedHelp({ isVisible }: { isVisible: boolean }) {
    const router = useRouter()
    if (!isVisible) return null
    const openEmailDomainStep = () => router.push(EMAIL_DOMAIN_STEP_HREF)
    return (
        <Button
            variant="link"
            className="self-start px-0"
            onPress={openEmailDomainStep}
            testID="setup-address-check-dns"
        >
            <ButtonText>Check your DNS records</ButtonText>
        </Button>
    )
}

const STATUS_COLOR: Record<TestMessageState['kind'], string> = {
    none: 'text-muted-foreground',
    sending: 'text-muted-foreground',
    sent: 'text-muted-foreground',
    delivered: 'text-success',
    bounced: 'text-danger',
}

function TestMessageStatus({ senderEmail }: { senderEmail: string }) {
    const state = useTestMessageState(senderEmail)
    if (state.kind === 'none') return null
    const labelClass = `text-sm ${STATUS_COLOR[state.kind]}`
    return (
        <View className="gap-1">
            <Text testID="setup-address-test-status" className={labelClass}>
                {testMessageLabel(state)}
            </Text>
            <BouncedHelp isVisible={state.kind === 'bounced'} />
        </View>
    )
}

function ErrorText({ message }: { message: string | null }) {
    if (!message) return null
    return <Text className="text-sm text-danger">{message}</Text>
}

function CreatedMailbox({ mailbox }: { mailbox: { id: string; email: string } }) {
    const { to, send, isPending, errorMessage } = useTestMessage(mailbox.id)
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
            </View>
            <TestMessageStatus senderEmail={mailbox.email} />
            <ErrorText message={errorMessage} />
        </View>
    )
}

// The user's username and name prefill the form, as the server does when it
// gives a later user their own address.
function useMe() {
    const [usersCollection] = useStore('users')
    const { data } = useMyLiveQuery((query, { userId }) =>
        query
            .from({ u: usersCollection })
            .where(({ u }) => eq(u.id, userId))
            .select(({ u }) => ({ id: u.id, username: u.username, name: u.name }))
    )
    return data?.[0]
}

function NewMailboxForm() {
    const me = useMe()
    const { rows } = useDomainRows()
    // The form reads its defaults once, so it mounts after the user row loads.
    if (!me) return null
    const defaults = { address: defaultAddressFor(me.username), display_name: me.name }
    return (
        <View className="mb-4">
            <MailboxForm
                mode="create"
                type="personal"
                domainOptions={verifiedDomainOptions(rows)}
                userId={me.id}
                defaults={defaults}
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
            <View className="flex-row items-center gap-2">
                <Text className="text-2xl font-bold text-foreground">Your email address</Text>
                <HelpIcon topic="mail:delivery-tracking" />
            </View>
            <Text className="mb-3 text-sm text-muted-foreground">
                Make the address you will send and receive email from.
            </Text>
            <AddressBody mailbox={mailbox} />
            <SetupContinueButton onPress={next} />
        </View>
    )
}
