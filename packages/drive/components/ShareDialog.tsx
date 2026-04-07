import { useLiveQuery } from '@tanstack/react-db'
import { ChevronDown, Link, Lock, Trash2 } from 'lucide-react-native'
import { useCallback, useMemo, useState } from 'react'
import { Platform, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native'
import { Button, Dialog, ListItem, SizableText, useTheme, YStack } from 'tamagui'
import { NameAvatar } from '~/components/NameAvatar'
import { pb, useStore } from '~/lib/pocketbase'
import { PlainInput } from '~/ui/PlainInput'

interface OrgMember {
    userOrgId: string
    name: string
    email: string
}

interface ShareEntry {
    id: string
    userOrgId: string
    name: string
    email: string
    role: string
}

interface PendingShare {
    key: string
    userOrgId: string
    name: string
    email: string
    role: 'editor' | 'viewer'
}

interface ShareDialogProps {
    open: boolean
    itemId: string
    itemName: string
    orgSlug: string
    shares: ShareEntry[]
    orgMembers: OrgMember[]
    currentUserOrgId: string
    onRemoveShare: (shareId: string) => void
    onClose: () => void
}

const webShadow =
    Platform.OS === 'web'
        ? ({ boxShadow: '0 4px 16px rgba(0,0,0,0.18)' } as Record<string, unknown>)
        : {}

export function ShareDialog({
    open,
    itemId,
    itemName,
    orgSlug,
    shares,
    orgMembers,
    currentUserOrgId,
    onRemoveShare,
    onClose,
}: ShareDialogProps) {
    const theme = useTheme()
    const [search, setSearch] = useState('')
    const [defaultRole, setDefaultRole] = useState<'editor' | 'viewer'>('editor')
    const [pending, setPending] = useState<PendingShare[]>([])
    const [linkCopied, setLinkCopied] = useState(false)
    const [isSaving, setIsSaving] = useState(false)

    const [contactsCollection] = useStore('contacts')
    const { data: contacts } = useLiveQuery(
        query =>
            query
                .from({ contacts: contactsCollection })
                .orderBy(({ contacts: c }) => c.first_name, 'asc'),
        []
    )

    const shareLink = useMemo(() => {
        if (!itemId || typeof window === 'undefined') return ''
        return `${window.location.origin}/a/${orgSlug}/drive?file=${itemId}`
    }, [itemId, orgSlug])

    const copyLink = useCallback(() => {
        if (Platform.OS === 'web') {
            navigator.clipboard.writeText(shareLink)
            setLinkCopied(true)
            setTimeout(() => setLinkCopied(false), 2000)
        }
    }, [shareLink])

    const alreadySharedIds = useMemo(() => new Set(shares.map(s => s.userOrgId)), [shares])
    const pendingEmails = useMemo(() => new Set(pending.map(p => p.email.toLowerCase())), [pending])

    const suggestions = useMemo(() => {
        if (search.length < 1) return []
        const q = search.toLowerCase()

        const memberResults = orgMembers
            .filter(
                m =>
                    !alreadySharedIds.has(m.userOrgId) &&
                    !pendingEmails.has(m.email.toLowerCase()) &&
                    m.userOrgId !== currentUserOrgId &&
                    (m.name.toLowerCase().includes(q) || m.email.toLowerCase().includes(q))
            )
            .map(m => ({
                key: `member:${m.userOrgId}`,
                userOrgId: m.userOrgId,
                name: m.name,
                email: m.email,
                source: 'member' as const,
            }))

        const memberEmails = new Set(orgMembers.map(m => m.email.toLowerCase()))
        const contactResults = (contacts ?? [])
            .filter(c => {
                if (!c.email) return false
                if (memberEmails.has(c.email.toLowerCase())) return false
                if (pendingEmails.has(c.email.toLowerCase())) return false
                const fullName = `${c.first_name} ${c.last_name}`.toLowerCase()
                return fullName.includes(q) || c.email.toLowerCase().includes(q)
            })
            .slice(0, 5)
            .map(c => ({
                key: `contact:${c.id}`,
                userOrgId: '',
                name: `${c.first_name} ${c.last_name}`.trim(),
                email: c.email,
                source: 'contact' as const,
            }))

        return [...memberResults, ...contactResults]
    }, [search, orgMembers, contacts, alreadySharedIds, pendingEmails, currentUserOrgId])

    const handleSelect = (s: (typeof suggestions)[number]) => {
        setPending(prev => [
            ...prev,
            {
                key: s.key,
                userOrgId: s.userOrgId,
                name: s.name,
                email: s.email,
                role: defaultRole,
            },
        ])
        setSearch('')
    }

    const removePending = (key: string) => {
        setPending(prev => prev.filter(p => p.key !== key))
    }

    const setPendingRole = (key: string, role: 'editor' | 'viewer') => {
        setPending(prev => prev.map(p => (p.key === key ? { ...p, role } : p)))
    }

    const handleDone = async () => {
        if (pending.length > 0) {
            setIsSaving(true)
            try {
                await pb.send('/api/drive/share', {
                    method: 'POST',
                    body: {
                        item_id: itemId,
                        recipients: pending.map(p => ({
                            user_org_id: p.userOrgId || undefined,
                            email: p.email,
                            name: p.name,
                            role: p.role,
                        })),
                    },
                })
            } catch {
                // Shares may have partially succeeded — close anyway
            } finally {
                setIsSaving(false)
            }
        }
        setPending([])
        setSearch('')
        onClose()
    }

    const currentUserShare = shares.find(s => s.userOrgId === currentUserOrgId)
    const otherShares = shares.filter(s => s.userOrgId !== currentUserOrgId)

    return (
        <Dialog
            modal
            open={open}
            onOpenChange={o => {
                if (!o) onClose()
            }}
        >
            <Dialog.Portal>
                <Dialog.Overlay
                    key="overlay"
                    opacity={0.3}
                    backgroundColor="$shadow6"
                    enterStyle={{ opacity: 0 }}
                    exitStyle={{ opacity: 0 }}
                />
                <Dialog.Content
                    key="content"
                    bordered
                    elevate
                    padding={0}
                    width={540}
                    backgroundColor="$background"
                    borderRadius={16}
                >
                    {/* Title */}
                    <View style={styles.header}>
                        <Text style={[styles.title, { color: theme.color.val }]}>
                            Share &ldquo;{itemName}&rdquo;
                        </Text>
                    </View>

                    {/* Search input */}
                    <View style={styles.inputSection}>
                        <View
                            style={[
                                styles.inputBorder,
                                { borderColor: theme.accentBackground.val },
                            ]}
                        >
                            <PlainInput
                                value={search}
                                onChangeText={setSearch}
                                placeholder="Add people by name or email"
                                placeholderTextColor={theme.accentBackground.val}
                                style={[styles.input, { color: theme.color.val }]}
                                autoFocus
                            />
                            <RolePicker value={defaultRole} onChange={setDefaultRole} />
                        </View>

                        {suggestions.length > 0 && (
                            <YStack
                                position="absolute"
                                top="100%"
                                left={0}
                                right={0}
                                zIndex={2000}
                                marginTop={2}
                                borderWidth={1}
                                borderRadius={12}
                                borderColor="$borderColor"
                                backgroundColor="$background"
                                overflow="hidden"
                                {...(webShadow as object)}
                            >
                                <ScrollView
                                    style={styles.suggestionScroll}
                                    keyboardShouldPersistTaps="handled"
                                >
                                    {suggestions.map(s => {
                                        const firstName =
                                            s.name.split(' ')[0] || s.email.split('@')[0]
                                        const lastName = s.name.split(' ').slice(1).join(' ')

                                        return (
                                            <ListItem
                                                key={s.key}
                                                size="$4"
                                                icon={
                                                    <NameAvatar
                                                        firstName={firstName}
                                                        lastName={lastName}
                                                        size={40}
                                                    />
                                                }
                                                title={
                                                    <SizableText size="$3" fontWeight="500">
                                                        {s.name || s.email}
                                                    </SizableText>
                                                }
                                                subTitle={
                                                    <SizableText size="$2" color="$color8">
                                                        {s.email}
                                                    </SizableText>
                                                }
                                                onPress={() => handleSelect(s)}
                                                hoverStyle={{
                                                    backgroundColor: '$backgroundHover',
                                                }}
                                                pressStyle={{
                                                    backgroundColor: '$backgroundPress',
                                                }}
                                                cursor="pointer"
                                                gap="$2"
                                            />
                                        )
                                    })}
                                </ScrollView>
                            </YStack>
                        )}
                    </View>

                    {/* Pending shares (just added, not yet saved) */}
                    {pending.length > 0 && (
                        <View style={styles.section}>
                            {pending.map(p => (
                                <View key={p.key} style={styles.personRow}>
                                    <NameAvatar firstName={p.name || p.email} size={36} />
                                    <View style={styles.personInfo}>
                                        <Text
                                            style={[styles.personName, { color: theme.color.val }]}
                                            numberOfLines={1}
                                        >
                                            {p.name || p.email}
                                        </Text>
                                        <Text
                                            style={[
                                                styles.personEmail,
                                                { color: theme.color8.val },
                                            ]}
                                            numberOfLines={1}
                                        >
                                            {p.email}
                                        </Text>
                                    </View>
                                    <RolePicker
                                        value={p.role}
                                        onChange={role => setPendingRole(p.key, role)}
                                    />
                                    <Pressable
                                        onPress={() => removePending(p.key)}
                                        style={styles.removeBtn}
                                    >
                                        <Trash2 size={14} color={theme.color8.val} />
                                    </Pressable>
                                </View>
                            ))}
                        </View>
                    )}

                    {/* People with access */}
                    <View style={styles.section}>
                        <Text style={[styles.sectionTitle, { color: theme.color.val }]}>
                            People with access
                        </Text>

                        {currentUserShare && (
                            <View style={styles.personRow}>
                                <NameAvatar
                                    firstName={currentUserShare.name || currentUserShare.email}
                                    size={36}
                                />
                                <View style={styles.personInfo}>
                                    <Text style={[styles.personName, { color: theme.color.val }]}>
                                        {currentUserShare.name || currentUserShare.email} (you)
                                    </Text>
                                    <Text style={[styles.personEmail, { color: theme.color8.val }]}>
                                        {currentUserShare.email}
                                    </Text>
                                </View>
                                <Text style={[styles.roleText, { color: theme.color8.val }]}>
                                    Owner
                                </Text>
                            </View>
                        )}

                        {otherShares.map(share => (
                            <View key={share.id} style={styles.personRow}>
                                <NameAvatar firstName={share.name || share.email} size={36} />
                                <View style={styles.personInfo}>
                                    <Text
                                        style={[styles.personName, { color: theme.color.val }]}
                                        numberOfLines={1}
                                    >
                                        {share.name || share.email}
                                    </Text>
                                    <Text
                                        style={[styles.personEmail, { color: theme.color8.val }]}
                                        numberOfLines={1}
                                    >
                                        {share.email}
                                    </Text>
                                </View>
                                <Text style={[styles.roleText, { color: theme.color8.val }]}>
                                    {share.role}
                                </Text>
                                <Pressable
                                    onPress={() => onRemoveShare(share.id)}
                                    style={styles.removeBtn}
                                >
                                    <Trash2 size={14} color={theme.color8.val} />
                                </Pressable>
                            </View>
                        ))}
                    </View>

                    {/* General access */}
                    <View style={styles.section}>
                        <Text style={[styles.sectionTitle, { color: theme.color.val }]}>
                            General access
                        </Text>
                        <View style={styles.personRow}>
                            <View
                                style={[
                                    styles.lockIcon,
                                    { backgroundColor: theme.backgroundHover.val },
                                ]}
                            >
                                <Lock size={16} color={theme.color8.val} />
                            </View>
                            <View style={styles.personInfo}>
                                <Text style={[styles.personName, { color: theme.color.val }]}>
                                    Restricted
                                </Text>
                                <Text style={[styles.personEmail, { color: theme.color8.val }]}>
                                    Only people with access can open with the link
                                </Text>
                            </View>
                        </View>
                    </View>

                    {/* Footer */}
                    <View style={[styles.footer, { borderTopColor: theme.borderColor.val }]}>
                        <Pressable
                            style={[styles.copyLinkButton, { borderColor: theme.borderColor.val }]}
                            onPress={copyLink}
                        >
                            <Link size={16} color={theme.accentBackground.val} />
                            <Text
                                style={[
                                    styles.copyLinkLabel,
                                    { color: theme.accentBackground.val },
                                ]}
                            >
                                {linkCopied ? 'Copied!' : 'Copy link'}
                            </Text>
                        </Pressable>
                        <Button
                            size="$4"
                            theme="accent"
                            borderRadius={24}
                            onPress={handleDone}
                            disabled={isSaving}
                        >
                            <Button.Text fontWeight="600" fontSize={14}>
                                {isSaving ? 'Saving...' : 'Done'}
                            </Button.Text>
                        </Button>
                    </View>
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog>
    )
}

function RolePicker({
    value,
    onChange,
}: {
    value: 'editor' | 'viewer'
    onChange: (role: 'editor' | 'viewer') => void
}) {
    const theme = useTheme()

    return (
        <Pressable
            style={[styles.rolePicker, { borderColor: theme.borderColor.val }]}
            onPress={() => onChange(value === 'editor' ? 'viewer' : 'editor')}
        >
            <Text style={[styles.rolePickerText, { color: theme.color.val }]}>
                {value === 'editor' ? 'Editor' : 'Viewer'}
            </Text>
            <ChevronDown size={14} color={theme.color8.val} />
        </Pressable>
    )
}

const styles = StyleSheet.create({
    header: {
        paddingHorizontal: 24,
        paddingTop: 28,
        paddingBottom: 16,
    },
    title: {
        fontSize: 22,
        fontWeight: '400',
    },
    inputSection: {
        paddingHorizontal: 24,
        paddingBottom: 20,
        position: 'relative',
        zIndex: 100,
        overflow: 'visible',
    },
    inputBorder: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 8,
        borderWidth: 2,
        borderRadius: 8,
        paddingHorizontal: 14,
        paddingVertical: 14,
    },
    input: {
        flex: 1,
        fontSize: 15,
    },
    suggestionScroll: {
        maxHeight: 300,
    },
    section: {
        paddingHorizontal: 24,
        paddingBottom: 16,
    },
    sectionTitle: {
        fontSize: 15,
        fontWeight: '600',
        marginBottom: 12,
    },
    personRow: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 12,
        paddingVertical: 6,
    },
    personInfo: {
        flex: 1,
        gap: 1,
    },
    personName: {
        fontSize: 14,
        fontWeight: '500',
    },
    personEmail: {
        fontSize: 13,
    },
    roleText: {
        fontSize: 13,
        textTransform: 'capitalize',
    },
    removeBtn: {
        padding: 6,
    },
    lockIcon: {
        width: 36,
        height: 36,
        borderRadius: 18,
        alignItems: 'center',
        justifyContent: 'center',
    },
    footer: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        paddingHorizontal: 24,
        paddingVertical: 16,
        borderTopWidth: 1,
    },
    copyLinkButton: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 8,
        paddingHorizontal: 16,
        paddingVertical: 10,
        borderRadius: 24,
        borderWidth: 1,
    },
    copyLinkLabel: {
        fontSize: 14,
        fontWeight: '600',
    },
    rolePicker: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 4,
        paddingHorizontal: 8,
        paddingVertical: 4,
        borderRadius: 6,
        borderWidth: 1,
    },
    rolePickerText: {
        fontSize: 13,
    },
})
