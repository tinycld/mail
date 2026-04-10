import { Pencil, Tag, Trash2 } from 'lucide-react-native'
import { useState } from 'react'
import { Pressable, StyleSheet, View } from 'react-native'
import { Button, H4, ScrollView, Separator, SizableText, useTheme, XStack, YStack } from 'tamagui'
import { LabelDialog } from '~/components/LabelDialog'
import { useLabelMutations } from '~/hooks/useLabelMutations'
import { useLabels } from '~/hooks/useLabels'
import { TextInput, useForm, z, zodResolver } from '~/ui/form'

const LABEL_COLORS = [
    '#e53935',
    '#d81b60',
    '#8e24aa',
    '#5e35b1',
    '#3949ab',
    '#1e88e5',
    '#00acc1',
    '#00897b',
    '#43a047',
    '#f4511e',
]

const createLabelSchema = z.object({
    name: z.string().min(1, 'Name is required').max(50, 'Name must be 50 characters or fewer'),
    color: z.string().min(1, 'Pick a color'),
})

function LabelRow({
    label,
    onEdit,
}: {
    label: { id: string; name: string; color: string }
    onEdit: () => void
}) {
    const theme = useTheme()
    const { deleteLabel } = useLabelMutations()
    const [confirmDelete, setConfirmDelete] = useState(false)

    const handleDelete = () => {
        deleteLabel.mutate(label.id)
    }

    return (
        <XStack
            alignItems="center"
            paddingHorizontal="$3"
            paddingVertical="$2"
            gap="$3"
            borderBottomWidth={1}
            borderBottomColor="$borderColor"
        >
            <View style={[styles.colorDot, { backgroundColor: label.color }]} />
            <SizableText flex={1} size="$3">
                {label.name}
            </SizableText>
            <Pressable onPress={onEdit} style={styles.iconButton}>
                <Pencil size={16} color={theme.color8.val} />
            </Pressable>
            {confirmDelete ? (
                <XStack gap="$2" alignItems="center">
                    <SizableText size="$2" color="$red10">
                        Delete?
                    </SizableText>
                    <Button
                        size="$2"
                        theme="red"
                        onPress={handleDelete}
                        disabled={deleteLabel.isPending}
                    >
                        Yes
                    </Button>
                    <Button size="$2" chromeless onPress={() => setConfirmDelete(false)}>
                        No
                    </Button>
                </XStack>
            ) : (
                <Pressable onPress={() => setConfirmDelete(true)} style={styles.iconButton}>
                    <Trash2 size={16} color={theme.color8.val} />
                </Pressable>
            )}
        </XStack>
    )
}

export default function LabelsSettings() {
    const theme = useTheme()
    const { labels } = useLabels()
    const { createLabel } = useLabelMutations()
    const [editingLabel, setEditingLabel] = useState<{
        id: string
        name: string
        color: string
    } | null>(null)
    const { control, handleSubmit, reset, setValue, watch } = useForm({
        resolver: zodResolver(createLabelSchema),
        mode: 'onChange',
        defaultValues: { name: '', color: LABEL_COLORS[0] },
    })

    const selectedColor = watch('color')

    const onSubmit = handleSubmit(data => {
        createLabel.mutate(data, {
            onSuccess: () => reset(),
        })
    })

    return (
        <ScrollView>
            <YStack padding="$4" gap="$4" maxWidth={600}>
                <XStack alignItems="center" gap="$3">
                    <Tag size={24} color={theme.color.val} />
                    <H4>Labels</H4>
                </XStack>

                <SizableText size="$3" color="$color8">
                    Manage labels for organizing your emails. Labels can be applied to threads from
                    the toolbar or email detail view.
                </SizableText>

                <Separator />

                <YStack gap="$0">
                    {labels.length === 0 ? (
                        <YStack padding="$4" alignItems="center">
                            <SizableText color="$color8">No labels yet</SizableText>
                        </YStack>
                    ) : (
                        labels.map(label => (
                            <LabelRow
                                key={label.id}
                                label={label}
                                onEdit={() => setEditingLabel(label)}
                            />
                        ))
                    )}
                </YStack>

                <Separator />

                <YStack gap="$3">
                    <SizableText size="$4" fontWeight="600">
                        Create Label
                    </SizableText>

                    <TextInput
                        control={control}
                        name="name"
                        label="Name"
                        placeholder="Label name"
                    />

                    <SizableText size="$2" color="$color8">
                        Color
                    </SizableText>
                    <XStack flexWrap="wrap" gap="$2">
                        {LABEL_COLORS.map(color => (
                            <Pressable
                                key={color}
                                style={[
                                    styles.colorSwatch,
                                    { backgroundColor: color },
                                    selectedColor === color && styles.colorSwatchSelected,
                                ]}
                                onPress={() => setValue('color', color)}
                            />
                        ))}
                    </XStack>

                    <Button
                        theme="accent"
                        onPress={onSubmit}
                        disabled={createLabel.isPending}
                        alignSelf="flex-start"
                    >
                        {createLabel.isPending ? 'Creating...' : 'Create Label'}
                    </Button>
                </YStack>
            </YStack>
            <LabelDialog
                isVisible={!!editingLabel}
                onClose={() => setEditingLabel(null)}
                label={editingLabel ?? undefined}
            />
        </ScrollView>
    )
}

const styles = StyleSheet.create({
    colorDot: {
        width: 12,
        height: 12,
        borderRadius: 6,
    },
    iconButton: {
        padding: 6,
    },
    colorSwatch: {
        width: 28,
        height: 28,
        borderRadius: 14,
    },
    colorSwatchSelected: {
        borderWidth: 3,
        borderColor: 'white',
        shadowColor: '#000',
        shadowOffset: { width: 0, height: 0 },
        shadowOpacity: 0.3,
        shadowRadius: 4,
    },
})
