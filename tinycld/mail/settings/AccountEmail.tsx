import { useStore } from '@tinycld/core/lib/pocketbase'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { ScrollView, Text, View } from 'react-native'
import { toAddressItems } from './address-items'
import { DefaultFromPicker } from './DefaultFromPicker'
import { MailAppLink } from './MailAppLink'
import { personEmailQuery, toPersonEmail } from './usePersonEmail'

type PersonEmail = ReturnType<typeof toPersonEmail>
type Identity = NonNullable<PersonEmail['identity']>

function useAccountEmail() {
    const [identities, addresses, domains, groups] = useStore(
        'mail_identities',
        'mail_addresses',
        'mail_domains',
        'groups'
    )
    const { data, isReady } = useMyLiveQuery(
        (query, { userId }) =>
            personEmailQuery(query, { identities, addresses, domains, groups }, userId),
        { queryKey: ['mail_account_email'] }
    )
    return { ...toPersonEmail(data ?? []), isReady }
}

function Note({ isVisible, text }: { isVisible: boolean; text: string }) {
    if (!isVisible) return null
    return <Text className="text-muted-foreground text-sm">{text}</Text>
}

function AddressList({ addresses }: { addresses: PersonEmail['addresses'] }) {
    return (
        <View className="gap-2">
            {toAddressItems(addresses).map(item => (
                <View key={item.id} className="flex-row items-center gap-3">
                    <Text className="flex-1 text-foreground text-sm">{item.email}</Text>
                    <Text className="text-muted-foreground text-xs">{item.badge}</Text>
                </View>
            ))}
        </View>
    )
}

function IdentitySection({
    isVisible,
    identity,
    addresses,
}: {
    isVisible: boolean
    identity: Identity | null
    addresses: PersonEmail['addresses']
}) {
    if (!isVisible || !identity) return null
    return (
        <View className="gap-4">
            <AddressList addresses={addresses} />
            <Note isVisible={addresses.length === 0} text="No address yet." />
            <DefaultFromPicker
                identityId={identity.id}
                addresses={addresses}
                value={identity.defaultFrom}
            />
            <MailAppLink />
        </View>
    )
}

/** Settings → Account → Email: the signed-in person's addresses and Default From. */
export default function AccountEmail() {
    const { identity, addresses, isReady } = useAccountEmail()
    const isRetired = identity?.retired === true
    return (
        <ScrollView contentContainerClassName="gap-4 p-4">
            <Note
                isVisible={isReady && identity === null}
                text="Guests do not get email addresses."
            />
            <IdentitySection isVisible={!isRetired} identity={identity} addresses={addresses} />
        </ScrollView>
    )
}
