import { and, eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import type { MemberDrawerPanelProps } from '@tinycld/core/lib/members/member-panel'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import { DefaultFromPicker } from './DefaultFromPicker'
import { EmailNameForm } from './EmailNameForm'
import { MailAppLink } from './MailAppLink'
import { MailboxAddressesPanel } from './MailboxAddressesPanel'
import { usePersonEmail } from './usePersonEmail'

type PersonEmail = ReturnType<typeof usePersonEmail>
type Identity = NonNullable<PersonEmail['identity']>

function useSharedInboxes(userId: string) {
    const [membersCollection, mailboxesCollection] = useStore(
        'mail_mailbox_members',
        'mail_mailboxes'
    )
    const { data } = useLiveQuery({
        queryKey: ['mail_person_shared_inboxes', userId],
        query: query =>
            query
                .from({ member: membersCollection })
                .innerJoin({ mailbox: mailboxesCollection }, ({ member, mailbox }) =>
                    eq(member.mailbox, mailbox.id)
                )
                .where(({ member, mailbox }) =>
                    and(eq(member.user, userId), eq(mailbox.type, 'shared'))
                )
                .select(({ mailbox }) => ({
                    id: mailbox.id,
                    displayName: mailbox.display_name,
                    name: mailbox.name,
                    address: mailbox.address,
                })),
    })
    // Labels are picked in JS: inside select() the fields are refs, always truthy.
    return (data ?? []).map(m => ({ id: m.id, name: m.displayName || m.name || m.address }))
}

function Heading() {
    return (
        <Text className="text-[12px] font-semibold text-muted uppercase tracking-wide">Email</Text>
    )
}

function Note({ isVisible, text }: { isVisible: boolean; text: string }) {
    if (!isVisible) return null
    return <Text className="text-muted-foreground text-sm">{text}</Text>
}

function EmailNameRow({
    identity,
    addresses,
}: {
    identity: Identity
    addresses: PersonEmail['addresses']
}) {
    const [isEditing, setIsEditing] = useState(false)
    return (
        <View className="gap-2">
            <View className="flex-row items-center justify-between">
                <Text className="text-foreground text-sm">{identity.emailName}</Text>
                <Pressable onPress={() => setIsEditing(s => !s)}>
                    <Text className="text-primary text-xs font-semibold">
                        {isEditing ? 'Cancel' : 'Edit'}
                    </Text>
                </Pressable>
            </View>
            <EmailNameFormSlot
                isVisible={isEditing}
                identity={identity}
                addresses={addresses}
                onDone={() => setIsEditing(false)}
            />
        </View>
    )
}

function EmailNameFormSlot({
    isVisible,
    identity,
    addresses,
    onDone,
}: {
    isVisible: boolean
    identity: Identity
    addresses: PersonEmail['addresses']
    onDone: () => void
}) {
    if (!isVisible) return null
    return <EmailNameForm identity={identity} addresses={addresses} onDone={onDone} />
}

function SharedInboxList({ userId }: { userId: string }) {
    const inboxes = useSharedInboxes(userId)
    if (inboxes.length === 0) return null
    return (
        <View className="gap-1">
            <Text className="text-muted-foreground text-xs">Shared inboxes</Text>
            {inboxes.map(inbox => (
                <Text key={inbox.id} className="text-foreground text-sm">
                    {inbox.name}
                </Text>
            ))}
        </View>
    )
}

function IdentitySection({
    identity,
    addresses,
    userId,
}: {
    identity: Identity
    addresses: PersonEmail['addresses']
    userId: string
}) {
    return (
        <View className="gap-3">
            <MailboxAddressesPanel mailboxId={identity.inbox} />
            <Note isVisible={addresses.length === 0} text="No address yet." />
            <EmailNameRow identity={identity} addresses={addresses} />
            <DefaultFromPicker
                identityId={identity.id}
                addresses={addresses}
                value={identity.defaultFrom}
            />
            <SharedInboxList userId={userId} />
            <MailAppLink />
        </View>
    )
}

function IdentitySlot({
    isVisible,
    ...props
}: {
    isVisible: boolean
    identity: Identity | null
    addresses: PersonEmail['addresses']
    userId: string
}) {
    if (!isVisible || !props.identity) return null
    return <IdentitySection {...props} identity={props.identity} />
}

/** Contributed to core's member drawer: a person's addresses, email name and shared inboxes. */
export default function MemberEmailPanel({ userId }: MemberDrawerPanelProps) {
    const { identity, addresses, isReady } = usePersonEmail(userId)
    const isRetired = identity?.retired === true
    return (
        <View className="gap-3">
            <Heading />
            <Note
                isVisible={isReady && identity === null}
                text="Guests do not get email addresses."
            />
            <Note isVisible={isRetired} text="This person has left. Their addresses are removed." />
            <IdentitySlot
                isVisible={!isRetired}
                identity={identity}
                addresses={addresses}
                userId={userId}
            />
        </View>
    )
}
