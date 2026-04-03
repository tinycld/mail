import { useState } from 'react'
import { Link, type Href } from 'one'
import { Pressable } from 'react-native'
import { useLiveQuery } from '@tanstack/react-db'
import { Star } from 'lucide-react-native'
import { useStore } from '~/lib/pocketbase'
import { useOrgInfo } from '~/lib/use-org-info'
import { useMutation } from '~/lib/mutations'
import { YStack, XStack, SizableText, Input, useTheme } from 'tamagui'
import { ContactAvatar } from '../components/ContactAvatar'

export default function ContactListScreen() {
    const { orgSlug } = useOrgInfo()
    const [contactsCollection] = useStore('contacts')
    const [searchQuery, setSearchQuery] = useState('')
    const newContactHref = `/app/${orgSlug}/contacts/new` as Href

    const { data: contacts, isLoading } = useLiveQuery((query) =>
        query
            .from({ contacts: contactsCollection })
            .orderBy(({ contacts }) => contacts.first_name, 'asc'),
    )

    const toggleFavorite = useMutation({
        mutationFn: function* ({ id, currentFavorite }: { id: string; currentFavorite: boolean }) {
            yield contactsCollection.update(id, (draft) => {
                draft.favorite = !currentFavorite
            })
        },
    })

    const query = searchQuery.toLowerCase()
    const filteredContacts = query
        ? contacts?.filter((c) => {
              const fullName = `${c.first_name} ${c.last_name}`.toLowerCase()
              return (
                  fullName.includes(query) ||
                  (c.email?.toLowerCase().includes(query)) ||
                  (c.company?.toLowerCase().includes(query))
              )
          })
        : contacts

    const count = filteredContacts?.length ?? 0

    if (isLoading) {
        return (
            <YStack flex={1} padding="$5" backgroundColor="$background">
                <SizableText size="$4" color="$color8">
                    Loading contacts...
                </SizableText>
            </YStack>
        )
    }

    const isEmpty = !contacts || contacts.length === 0

    if (isEmpty) {
        return (
            <YStack flex={1} padding="$5" backgroundColor="$background">
                <SizableText size="$8" fontWeight="bold" color="$color">
                    Contacts
                </SizableText>
                <SizableText size="$4" color="$color8" marginTop="$2" marginBottom="$4">
                    No contacts yet.
                </SizableText>
                <Link href={newContactHref}>
                    <SizableText size="$4" color="$accentColor" fontWeight="600">
                        + Add Contact
                    </SizableText>
                </Link>
            </YStack>
        )
    }

    return (
        <YStack flex={1} padding="$5" backgroundColor="$background">
            <XStack justifyContent="space-between" alignItems="center" marginBottom="$4">
                <SizableText size="$7" fontWeight="bold" color="$color">
                    Contacts ({count})
                </SizableText>
                <Input
                    size="$3"
                    placeholder="Search contacts..."
                    value={searchQuery}
                    onChangeText={setSearchQuery}
                    width={250}
                    backgroundColor="$background"
                    borderColor="$borderColor"
                    placeholderTextColor="$placeholderColor"
                    color="$color"
                />
            </XStack>

            <XStack
                paddingHorizontal="$3"
                paddingVertical="$2"
                borderBottomWidth={1}
                borderBottomColor="$borderColor"
            >
                <SizableText size="$2" color="$color8" fontWeight="600" flex={2}>
                    Name
                </SizableText>
                <SizableText size="$2" color="$color8" fontWeight="600" flex={2}>
                    Email
                </SizableText>
                <SizableText size="$2" color="$color8" fontWeight="600" flex={1}>
                    Phone
                </SizableText>
                <YStack width={32} />
            </XStack>

            <YStack>
                {filteredContacts?.map((contact) => (
                    <ContactRow
                        key={contact.id}
                        contact={contact}
                        orgSlug={orgSlug}
                        onToggleFavorite={() =>
                            toggleFavorite.mutate({ id: contact.id, currentFavorite: contact.favorite })
                        }
                    />
                ))}
            </YStack>
        </YStack>
    )
}

interface ContactRowProps {
    contact: {
        id: string
        first_name: string
        last_name: string
        email: string
        phone: string
        favorite: boolean
    }
    orgSlug: string
    onToggleFavorite: () => void
}

function ContactRow({ contact, orgSlug, onToggleFavorite }: ContactRowProps) {
    const theme = useTheme()
    const displayName = [contact.first_name, contact.last_name].filter(Boolean).join(' ')

    return (
        <Link href={`/app/${orgSlug}/contacts/${contact.id}` as Href}>
            <XStack
                paddingHorizontal="$3"
                paddingVertical="$3"
                alignItems="center"
                borderBottomWidth={1}
                borderBottomColor="$borderColor"
                hoverStyle={{ backgroundColor: '$backgroundHover' }}
            >
                <XStack flex={2} alignItems="center" gap="$3">
                    <ContactAvatar firstName={contact.first_name} lastName={contact.last_name} />
                    <SizableText size="$4" color="$color" fontWeight="500" numberOfLines={1}>
                        {displayName}
                    </SizableText>
                </XStack>
                <SizableText size="$3" color="$color8" flex={2} numberOfLines={1}>
                    {contact.email}
                </SizableText>
                <SizableText size="$3" color="$color8" flex={1} numberOfLines={1}>
                    {contact.phone}
                </SizableText>
                <Pressable
                    onPress={(e) => {
                        e.stopPropagation()
                        onToggleFavorite()
                    }}
                >
                    <Star
                        size={18}
                        color={contact.favorite ? theme.yellow8.val : theme.color8.val}
                        fill={contact.favorite ? theme.yellow8.val : 'transparent'}
                    />
                </Pressable>
            </XStack>
        </Link>
    )
}
