import { useStore } from '@tinycld/core/lib/pocketbase'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useMyLiveQuery } from '@tinycld/core/lib/use-my-live-query'
import { ScrollView, Text, View } from 'react-native'
import { type AddressItem, toAddressItems } from './address-items'
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

function AddressRow({ item }: { item: AddressItem }) {
    return (
        <View className="flex-row items-center gap-3">
            <Text className="flex-1 text-foreground text-sm">{item.email}</Text>
            <Text className="text-muted-foreground text-xs">{item.badge}</Text>
        </View>
    )
}

function AddressList({ addresses }: { addresses: PersonEmail['addresses'] }) {
    const rows = toAddressItems(addresses).map(item => <AddressRow key={item.id} item={item} />)
    return <View className="gap-2">{rows}</View>
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
    const role = useCurrentRole()
    const isRetired = identity?.retired === true
    const hasNoIdentity = isReady && role.isReady && identity === null
    return (
        <ScrollView contentContainerClassName="gap-4 p-4">
            <Note
                isVisible={hasNoIdentity && role.isGuest}
                text="Guests do not get email addresses."
            />
            <Note isVisible={hasNoIdentity && !role.isGuest} text="You have no email yet." />
            <IdentitySection isVisible={!isRetired} identity={identity} addresses={addresses} />
        </ScrollView>
    )
}
