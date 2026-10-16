import { eq } from '@tanstack/db'
import { useLiveQuery } from '@tanstack/react-db'
import { mutation, useMutation } from '@tinycld/core/lib/mutations'
import { useStore } from '@tinycld/core/lib/pocketbase'
import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import { useServerActionState } from '@tinycld/core/lib/use-writes-available'
import { AtSign, Plus, X } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, Text, View } from 'react-native'
import { AddAddressForm } from './AddAddressForm'
import { type AddressItem, toAddressItems } from './address-items'

function useMailboxAddresses(mailboxId: string) {
    const [addressesCollection, domainsCollection] = useStore('mail_addresses', 'mail_domains')
    const { data } = useLiveQuery(query =>
        query
            .from({ address: addressesCollection })
            .innerJoin({ domain: domainsCollection }, ({ address, domain }) =>
                eq(address.domain, domain.id)
            )
            .where(({ address }) => eq(address.mailbox, mailboxId))
            .select(({ address, domain }) => ({
                id: address.id,
                localPart: address.local_part,
                domainName: domain.domain,
                source: address.source,
                isPrimary: address.is_primary,
                rank: domain.rank,
            }))
    )
    const remove = useMutation({
        mutationFn: mutation(function* (addressId: string) {
            yield addressesCollection.delete(addressId)
        }),
    })
    return { items: toAddressItems(data ?? []), remove }
}

export function MailboxAddressesPanel({ mailboxId }: { mailboxId: string }) {
    const primaryColor = useThemeColor('primary')
    const [isAdding, setIsAdding] = useState(false)
    const { items, remove } = useMailboxAddresses(mailboxId)
    const toggleLabel = isAdding ? 'Cancel' : 'Add address'

    return (
        <View className="gap-3">
            <View className="flex-row items-center justify-between">
                <Text
                    className="text-muted-foreground uppercase"
                    style={{ fontSize: 11, fontWeight: '600', letterSpacing: 0.6 }}
                >
                    Addresses
                </Text>
                <Pressable
                    onPress={() => setIsAdding(s => !s)}
                    className="flex-row gap-1 items-center"
                >
                    <Plus size={12} color={primaryColor} />
                    <Text className="text-primary" style={{ fontSize: 12, fontWeight: '600' }}>
                        {toggleLabel}
                    </Text>
                </Pressable>
            </View>

            {items.map(item => (
                <AddressRow key={item.id} item={item} onRemove={() => remove.mutate(item.id)} />
            ))}

            <AddFormSlot
                isVisible={isAdding}
                mailboxId={mailboxId}
                onDone={() => setIsAdding(false)}
            />
        </View>
    )
}

function AddFormSlot({
    isVisible,
    mailboxId,
    onDone,
}: {
    isVisible: boolean
    mailboxId: string
    onDone: () => void
}) {
    if (!isVisible) return null
    return <AddAddressForm mailboxId={mailboxId} onDone={onDone} />
}

function AddressRow({ item, onRemove }: { item: AddressItem; onRemove: () => void }) {
    const primaryColor = useThemeColor('primary')
    return (
        <View className="flex-row items-center gap-3 rounded-lg py-2">
            <View
                className="items-center justify-center rounded-lg bg-primary/10"
                style={{ width: 28, height: 28 }}
            >
                <AtSign size={14} color={primaryColor} />
            </View>
            <Text className="flex-1 text-foreground" style={{ fontSize: 13 }}>
                {item.email}
            </Text>
            <Badge label={item.badge} />
            <RemoveButton isVisible={item.canRemove} email={item.email} onRemove={onRemove} />
        </View>
    )
}

function Badge({ label }: { label: string | null }) {
    if (!label) return null
    return (
        <Text className="text-muted-foreground" style={{ fontSize: 11 }}>
            {label}
        </Text>
    )
}

function RemoveButton({
    isVisible,
    email,
    onRemove,
}: {
    isVisible: boolean
    email: string
    onRemove: () => void
}) {
    const dangerColor = useThemeColor('danger')
    const state = useServerActionState({ accessibilityHint: `Removes the address ${email}` })
    if (!isVisible) return null
    return (
        <Pressable
            onPress={onRemove}
            disabled={state.isDisabled}
            accessibilityRole="button"
            accessibilityLabel={`Remove address ${email}`}
            accessibilityHint={state.accessibilityHint}
            className="p-1"
            style={{ opacity: state.isDisabled ? 0.4 : 1 }}
        >
            <X size={14} color={dangerColor} />
        </Pressable>
    )
}
