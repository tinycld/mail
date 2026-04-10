import { X } from 'lucide-react-native'
import { newRecordId } from 'pbtsdb'
import { Modal, Pressable, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useMutation } from '~/lib/mutations'
import { useStore } from '~/lib/pocketbase'
import { useOrgInfo } from '~/lib/use-org-info'
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

const labelSchema = z.object({
    name: z.string().min(1, 'Name is required').max(50, 'Name must be 50 characters or fewer'),
    color: z.string().min(1, 'Pick a color'),
})

interface LabelCreateDialogProps {
    isVisible: boolean
    onClose: () => void
}

export function LabelCreateDialog({ isVisible, onClose }: LabelCreateDialogProps) {
    const theme = useTheme()
    const { orgId } = useOrgInfo()
    const [labelsCollection] = useStore('labels')

    const {
        control,
        handleSubmit,
        reset,
        setValue,
        watch,
        formState: { errors },
    } = useForm({
        resolver: zodResolver(labelSchema),
        mode: 'onChange',
        defaultValues: { name: '', color: LABEL_COLORS[0] },
    })

    const selectedColor = watch('color')

    const createLabel = useMutation({
        mutationFn: function* (data: { name: string; color: string }) {
            yield labelsCollection.insert({
                id: newRecordId(),
                org: orgId,
                name: data.name,
                color: data.color,
            })
        },
        onSuccess: () => {
            reset()
            onClose()
        },
    })

    const onSubmit = handleSubmit(data => createLabel.mutate(data))

    const handleClose = () => {
        reset()
        onClose()
    }

    if (!isVisible) return null

    return (
        <Modal visible transparent animationType="fade" onRequestClose={handleClose}>
            <Pressable style={styles.backdrop} onPress={handleClose}>
                <Pressable
                    style={[
                        styles.dialog,
                        {
                            backgroundColor: theme.background.val,
                            borderColor: theme.borderColor.val,
                        },
                    ]}
                    onPress={e => e.stopPropagation()}
                >
                    <View style={styles.header}>
                        <Text style={[styles.title, { color: theme.color.val }]}>New label</Text>
                        <Pressable onPress={handleClose} style={styles.closeButton}>
                            <X size={18} color={theme.color8.val} />
                        </Pressable>
                    </View>

                    <View style={styles.body}>
                        <TextInput
                            control={control}
                            name="name"
                            label="Name"
                            placeholder="Label name"
                        />
                        {errors.name ? (
                            <Text style={[styles.errorText, { color: theme.red10.val }]}>
                                {errors.name.message}
                            </Text>
                        ) : null}

                        <Text style={[styles.colorLabel, { color: theme.color8.val }]}>Color</Text>
                        <View style={styles.colorGrid}>
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
                        </View>
                    </View>

                    <View style={styles.footer}>
                        <Pressable
                            style={[styles.cancelButton, { borderColor: theme.borderColor.val }]}
                            onPress={handleClose}
                        >
                            <Text style={{ color: theme.color.val }}>Cancel</Text>
                        </Pressable>
                        <Pressable
                            style={[
                                styles.createButton,
                                { backgroundColor: theme.accentBackground.val },
                            ]}
                            onPress={onSubmit}
                            disabled={createLabel.isPending}
                        >
                            <Text style={{ color: theme.accentColor.val }}>
                                {createLabel.isPending ? 'Creating...' : 'Create'}
                            </Text>
                        </Pressable>
                    </View>
                </Pressable>
            </Pressable>
        </Modal>
    )
}

const styles = StyleSheet.create({
    backdrop: {
        flex: 1,
        backgroundColor: 'rgba(0,0,0,0.4)',
        alignItems: 'center',
        justifyContent: 'center',
    },
    dialog: {
        width: 360,
        borderRadius: 12,
        borderWidth: 1,
    },
    header: {
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'space-between',
        paddingHorizontal: 16,
        paddingVertical: 12,
    },
    title: {
        fontSize: 16,
        fontWeight: '600',
    },
    closeButton: {
        padding: 4,
    },
    body: {
        paddingHorizontal: 16,
        gap: 8,
    },
    errorText: {
        fontSize: 12,
    },
    colorLabel: {
        fontSize: 13,
        fontWeight: '500',
        marginTop: 4,
    },
    colorGrid: {
        flexDirection: 'row',
        flexWrap: 'wrap',
        gap: 8,
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
    footer: {
        flexDirection: 'row',
        justifyContent: 'flex-end',
        gap: 8,
        padding: 16,
    },
    cancelButton: {
        paddingHorizontal: 16,
        paddingVertical: 8,
        borderRadius: 6,
        borderWidth: 1,
    },
    createButton: {
        paddingHorizontal: 16,
        paddingVertical: 8,
        borderRadius: 6,
    },
})
