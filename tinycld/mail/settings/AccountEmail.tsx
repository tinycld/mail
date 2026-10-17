import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { Switch } from '@tinycld/core/ui/switch'
import { ScrollView, Text, View } from 'react-native'
import { type AddressItem, toAddressItems } from './address-items'
import { DefaultFromPicker } from './DefaultFromPicker'
import { MailAppSetupLink } from './MailAppLink'
import type { toPersonEmail } from './usePersonEmail'
import { useMyPersonEmail } from './usePersonEmail'

type PersonEmail = ReturnType<typeof toPersonEmail>
type Identity = NonNullable<PersonEmail['identity']>

function Note({ isVisible, text }: { isVisible: boolean; text: string }) {
    if (!isVisible) return null
    return <Text className="text-muted-foreground text-sm">{text}</Text>
}

function nextHiddenAddressViews(hidden: string[], addressId: string, shown: boolean) {
    if (shown) return hidden.filter(id => id !== addressId)
    return hidden.includes(addressId) ? hidden : [...hidden, addressId]
}

// Each switch saves on change; a failed save reverts the optimistic row and
// useMutation's default error toast says why.
function useAddressViewSwitches(identityId: string) {
    const [identitiesCollection] = useStore('mail_identities')
    const setEnabled = useMutation({
        mutationFn: mutation(function* (enabled: boolean) {
            yield identitiesCollection.update(identityId, d => {
                d.address_views = enabled
            })
        }),
    })
    const setShown = useMutation({
        mutationFn: mutation(function* ({
            addressId,
            shown,
        }: {
            addressId: string
            shown: boolean
        }) {
            yield identitiesCollection.update(identityId, d => {
                d.hidden_address_views = nextHiddenAddressViews(
                    d.hidden_address_views,
                    addressId,
                    shown
                )
            })
        }),
    })
    return { setEnabled: setEnabled.mutate, setShown: setShown.mutate }
}

type SetShown = ReturnType<typeof useAddressViewSwitches>['setShown']

function ShowInSidebarSwitch({
    isVisible,
    item,
    isShown,
    onChange,
}: {
    isVisible: boolean
    item: AddressItem
    isShown: boolean
    onChange: SetShown
}) {
    if (!isVisible) return null
    return (
        <Switch
            value={isShown}
            onValueChange={shown => onChange({ addressId: item.id, shown })}
            accessibilityLabel={`Show ${item.email} in sidebar`}
        />
    )
}

function AddressRow({
    item,
    showsSwitch,
    isShown,
    onShownChange,
}: {
    item: AddressItem
    showsSwitch: boolean
    isShown: boolean
    onShownChange: SetShown
}) {
    return (
        <View className="flex-row items-center gap-3">
            <Text className="flex-1 text-foreground text-sm">{item.email}</Text>
            <Text className="text-muted-foreground text-xs">{item.badge}</Text>
            <ShowInSidebarSwitch
                isVisible={showsSwitch}
                item={item}
                isShown={isShown}
                onChange={onShownChange}
            />
        </View>
    )
}

// The column heading over the row switches, so each switch has a visible name.
function ShowInSidebarHeading({ isVisible }: { isVisible: boolean }) {
    if (!isVisible) return null
    return (
        <View className="flex-row justify-end">
            <Text className="text-muted-foreground text-xs">Show in sidebar</Text>
        </View>
    )
}

function AddressList({
    identity,
    addresses,
    showsSwitches,
    onShownChange,
}: {
    identity: Identity
    addresses: PersonEmail['addresses']
    showsSwitches: boolean
    onShownChange: SetShown
}) {
    const rows = toAddressItems(addresses).map(item => (
        <AddressRow
            key={item.id}
            item={item}
            showsSwitch={showsSwitches}
            isShown={!identity.hiddenAddressViews.includes(item.id)}
            onShownChange={onShownChange}
        />
    ))
    return (
        <View className="gap-2">
            <ShowInSidebarHeading isVisible={showsSwitches} />
            {rows}
        </View>
    )
}

function AddressViewsSwitch({
    isVisible,
    value,
    onChange,
}: {
    isVisible: boolean
    value: boolean
    onChange: (enabled: boolean) => void
}) {
    if (!isVisible) return null
    return (
        <View className="gap-1">
            <View className="flex-row items-center justify-between gap-2">
                <Text className="text-sm font-semibold text-foreground">
                    Show a folder for each address
                </Text>
                <Switch
                    value={value}
                    onValueChange={onChange}
                    accessibilityLabel="Show a folder for each address"
                />
            </View>
            <Text className="text-xs text-muted-foreground">
                The sidebar shows a folder under Inbox for each address. Your mail app shows these
                folders in the Addresses folder. Each folder shows the conversations that use that
                address.
            </Text>
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
    return <IdentityDetails identity={identity} addresses={addresses} />
}

function IdentityDetails({
    identity,
    addresses,
}: {
    identity: Identity
    addresses: PersonEmail['addresses']
}) {
    const { setEnabled, setShown } = useAddressViewSwitches(identity.id)
    // A folder of the only address would be the whole inbox, so the switch is
    // offered for two or more addresses; once on, it stays offered so it can
    // be turned off after addresses go away.
    const showsMainSwitch = addresses.length >= 2 || identity.addressViews
    const showsRowSwitches = showsMainSwitch && identity.addressViews
    return (
        <View className="gap-4">
            <AddressList
                identity={identity}
                addresses={addresses}
                showsSwitches={showsRowSwitches}
                onShownChange={setShown}
            />
            <Note isVisible={addresses.length === 0} text="No address yet." />
            <AddressViewsSwitch
                isVisible={showsMainSwitch}
                value={identity.addressViews}
                onChange={setEnabled}
            />
            <DefaultFromPicker
                identityId={identity.id}
                addresses={addresses}
                value={identity.defaultFrom}
            />
            <MailAppSetupLink />
        </View>
    )
}

/**
 * Settings → Account → Email: the signed-in person's addresses, their
 * Default From, and which addresses get a folder of their own.
 */
export default function AccountEmail() {
    const { identity, addresses, isReady } = useMyPersonEmail()
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
