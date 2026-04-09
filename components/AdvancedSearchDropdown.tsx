import { zodResolver } from '@hookform/resolvers/zod'
import { ChevronDown, Search, Square, SquareCheck, X } from 'lucide-react-native'
import { useCallback, useMemo, useState } from 'react'
import { useController, useForm } from 'react-hook-form'
import {
    Platform,
    Pressable,
    ScrollView,
    type StyleProp,
    StyleSheet,
    Text,
    type TextStyle,
    View,
} from 'react-native'
import { useTheme } from 'tamagui'
import { z } from 'zod'
import { PlainInput } from '~/ui/PlainInput'
import type { AdvancedSearchFilters } from '../hooks/useSearchState'

const schema = z.object({
    from: z.string(),
    to: z.string(),
    subject: z.string(),
    hasWords: z.string(),
    doesntHave: z.string(),
    sizeOp: z.enum(['greater_than', 'less_than']),
    sizeValue: z.string(),
    sizeUnit: z.enum(['MB', 'KB', 'bytes']),
    dateWithin: z.enum(['', '1d', '3d', '1w', '2w', '1m', '2m', '6m', '1y']),
    dateAnchor: z.string(),
    folder: z.enum(['all', 'inbox', 'sent', 'drafts', 'trash', 'spam', 'starred']),
    hasAttachment: z.boolean(),
})

type FormValues = z.infer<typeof schema>

function makeDefaultValues(): FormValues {
    return {
        from: '',
        to: '',
        subject: '',
        hasWords: '',
        doesntHave: '',
        sizeOp: 'greater_than',
        sizeValue: '',
        sizeUnit: 'MB',
        dateWithin: '',
        dateAnchor: new Date().toISOString().split('T')[0],
        folder: 'all',
        hasAttachment: false,
    }
}

function formDataToFilters(data: FormValues): AdvancedSearchFilters {
    const filters: AdvancedSearchFilters = {}
    const textFields = ['from', 'to', 'subject', 'hasWords', 'doesntHave'] as const
    for (const key of textFields) {
        if (data[key]) filters[key] = data[key]
    }
    if (data.sizeValue) {
        filters.sizeOp = data.sizeOp
        filters.sizeValue = data.sizeValue
        filters.sizeUnit = data.sizeUnit
    }
    if (data.dateWithin) {
        filters.dateWithin = data.dateWithin
        filters.dateAnchor = data.dateAnchor
    }
    if (data.folder !== 'all') filters.folder = data.folder
    if (data.hasAttachment) filters.hasAttachment = true
    return filters
}

interface AdvancedSearchDropdownProps {
    onApply: (filters: AdvancedSearchFilters) => void
    onClose: () => void
    initialFilters?: AdvancedSearchFilters
}

export function AdvancedSearchDropdown({
    onApply,
    onClose,
    initialFilters,
}: AdvancedSearchDropdownProps) {
    const theme = useTheme()

    const defaults = useMemo(
        () => ({ ...makeDefaultValues(), ...initialFilters }),
        [initialFilters]
    )

    const { control, handleSubmit, setValue, watch, reset } = useForm<FormValues>({
        resolver: zodResolver(schema),
        defaultValues: defaults,
    })

    const onSubmit = handleSubmit(data => {
        onApply(formDataToFilters(data))
        onClose()
    })

    const onClear = useCallback(() => {
        reset(makeDefaultValues())
        onApply({})
        onClose()
    }, [reset, onApply, onClose])

    const hasAttachment = watch('hasAttachment')

    const toggleAttachment = useCallback(() => {
        setValue('hasAttachment', !hasAttachment)
    }, [hasAttachment, setValue])

    const inputStyle = [
        styles.input,
        {
            color: theme.color.val,
            borderColor: theme.borderColor.val,
        },
    ]

    const labelColor = theme.color8.val
    const shadowColor = theme.shadowColor.val

    return (
        <>
            <Pressable style={styles.overlay} onPress={onClose} />
            <View
                style={[
                    styles.panel,
                    {
                        backgroundColor: theme.background.val,
                        borderColor: theme.borderColor.val,
                        ...Platform.select({
                            web: {
                                boxShadow: `0 4px 24px ${shadowColor}`,
                            },
                            default: {
                                shadowColor: shadowColor,
                                shadowOffset: { width: 0, height: 4 },
                                shadowOpacity: 0.15,
                                shadowRadius: 12,
                                elevation: 8,
                            },
                        }),
                    },
                ]}
            >
                <ScrollView style={styles.scrollContent} keyboardShouldPersistTaps="handled">
                    <FieldRow label="From" labelColor={labelColor}>
                        <FormInput
                            control={control}
                            name="from"
                            style={inputStyle}
                            placeholder=""
                        />
                    </FieldRow>
                    <FieldRow label="To" labelColor={labelColor}>
                        <FormInput control={control} name="to" style={inputStyle} placeholder="" />
                    </FieldRow>
                    <FieldRow label="Subject" labelColor={labelColor}>
                        <FormInput
                            control={control}
                            name="subject"
                            style={inputStyle}
                            placeholder=""
                        />
                    </FieldRow>
                    <FieldRow label="Has the words" labelColor={labelColor}>
                        <FormInput
                            control={control}
                            name="hasWords"
                            style={inputStyle}
                            placeholder=""
                        />
                    </FieldRow>
                    <FieldRow label="Doesn't have" labelColor={labelColor}>
                        <FormInput
                            control={control}
                            name="doesntHave"
                            style={inputStyle}
                            placeholder=""
                        />
                    </FieldRow>
                    <FieldRow label="Size" labelColor={labelColor}>
                        <View style={styles.compoundRow}>
                            <PickerButton
                                control={control}
                                name="sizeOp"
                                options={SIZE_OP_OPTIONS}
                                theme={theme}
                            />
                            <FormInput
                                control={control}
                                name="sizeValue"
                                style={[...inputStyle, styles.numberInput]}
                                placeholder=""
                                keyboardType="numeric"
                            />
                            <PickerButton
                                control={control}
                                name="sizeUnit"
                                options={SIZE_UNIT_OPTIONS}
                                theme={theme}
                            />
                        </View>
                    </FieldRow>
                    <FieldRow label="Date within" labelColor={labelColor}>
                        <View style={styles.compoundRow}>
                            <PickerButton
                                control={control}
                                name="dateWithin"
                                options={DATE_WITHIN_OPTIONS}
                                theme={theme}
                            />
                            <FormInput
                                control={control}
                                name="dateAnchor"
                                style={inputStyle}
                                placeholder="YYYY-MM-DD"
                            />
                        </View>
                    </FieldRow>
                    <FieldRow label="Search" labelColor={labelColor}>
                        <PickerButton
                            control={control}
                            name="folder"
                            options={FOLDER_OPTIONS}
                            theme={theme}
                        />
                    </FieldRow>
                    <FieldRow label="Has attachment" labelColor={labelColor}>
                        <Pressable onPress={toggleAttachment} style={styles.checkboxRow}>
                            {hasAttachment ? (
                                <SquareCheck size={20} color={theme.accentBackground.val} />
                            ) : (
                                <Square size={20} color={theme.color8.val} />
                            )}
                        </Pressable>
                    </FieldRow>
                </ScrollView>
                <View style={[styles.footer, { borderTopColor: theme.borderColor.val }]}>
                    <Pressable onPress={onClear} style={styles.clearButton}>
                        <X size={14} color={theme.color8.val} />
                        <Text style={[styles.clearButtonText, { color: theme.color8.val }]}>
                            Clear
                        </Text>
                    </Pressable>
                    <Pressable
                        onPress={onSubmit}
                        style={[
                            styles.searchButton,
                            { backgroundColor: theme.accentBackground.val },
                        ]}
                    >
                        <Search size={16} color={theme.accentColor.val} />
                        <Text style={[styles.searchButtonText, { color: theme.accentColor.val }]}>
                            Search
                        </Text>
                    </Pressable>
                </View>
            </View>
        </>
    )
}

function FieldRow({
    label,
    labelColor,
    children,
}: {
    label: string
    labelColor: string
    children: React.ReactNode
}) {
    return (
        <View style={styles.fieldRow}>
            <Text style={[styles.fieldLabel, { color: labelColor }]}>{label}</Text>
            <View style={styles.fieldValue}>{children}</View>
        </View>
    )
}

function FormInput({
    control,
    name,
    style,
    placeholder,
    keyboardType,
}: {
    control: ReturnType<typeof useForm<FormValues>>['control']
    name: keyof FormValues
    style: StyleProp<TextStyle>
    placeholder: string
    keyboardType?: 'numeric' | 'default'
}) {
    const { field } = useController({ control, name })

    return (
        <PlainInput
            style={style}
            value={String(field.value ?? '')}
            onChangeText={field.onChange}
            placeholder={placeholder}
            keyboardType={keyboardType}
        />
    )
}

interface PickerOption {
    label: string
    value: string
}

const SIZE_OP_OPTIONS: PickerOption[] = [
    { label: 'greater than', value: 'greater_than' },
    { label: 'less than', value: 'less_than' },
]

const SIZE_UNIT_OPTIONS: PickerOption[] = [
    { label: 'MB', value: 'MB' },
    { label: 'KB', value: 'KB' },
    { label: 'bytes', value: 'bytes' },
]

const DATE_WITHIN_OPTIONS: PickerOption[] = [
    { label: 'any time', value: '' },
    { label: '1 day', value: '1d' },
    { label: '3 days', value: '3d' },
    { label: '1 week', value: '1w' },
    { label: '2 weeks', value: '2w' },
    { label: '1 month', value: '1m' },
    { label: '2 months', value: '2m' },
    { label: '6 months', value: '6m' },
    { label: '1 year', value: '1y' },
]

const FOLDER_OPTIONS: PickerOption[] = [
    { label: 'All Mail', value: 'all' },
    { label: 'Inbox', value: 'inbox' },
    { label: 'Sent', value: 'sent' },
    { label: 'Drafts', value: 'drafts' },
    { label: 'Trash', value: 'trash' },
    { label: 'Spam', value: 'spam' },
    { label: 'Starred', value: 'starred' },
]

function PickerButton({
    control,
    name,
    options,
    theme,
}: {
    control: ReturnType<typeof useForm<FormValues>>['control']
    name: keyof FormValues
    options: PickerOption[]
    theme: ReturnType<typeof useTheme>
}) {
    const { field } = useController({ control, name })
    const [isOpen, setIsOpen] = useState(false)

    const selectedLabel = options.find(o => o.value === field.value)?.label ?? options[0].label

    return (
        <View style={styles.pickerContainer}>
            <Pressable
                onPress={() => setIsOpen(!isOpen)}
                style={[styles.pickerButton, { borderColor: theme.borderColor.val }]}
            >
                <Text style={[styles.pickerText, { color: theme.color.val }]} numberOfLines={1}>
                    {selectedLabel}
                </Text>
                <ChevronDown size={14} color={theme.color8.val} />
            </Pressable>
            {isOpen ? (
                <PickerMenu
                    options={options}
                    value={String(field.value)}
                    onSelect={val => {
                        field.onChange(val)
                        setIsOpen(false)
                    }}
                    onClose={() => setIsOpen(false)}
                    theme={theme}
                />
            ) : null}
        </View>
    )
}

function PickerMenu({
    options,
    value,
    onSelect,
    onClose,
    theme,
}: {
    options: PickerOption[]
    value: string
    onSelect: (value: string) => void
    onClose: () => void
    theme: ReturnType<typeof useTheme>
}) {
    return (
        <>
            <Pressable
                style={[styles.pickerOverlay, Platform.OS === 'web' && styles.pickerOverlayWeb]}
                onPress={onClose}
            />
            <View
                style={[
                    styles.pickerMenu,
                    {
                        backgroundColor: theme.background.val,
                        borderColor: theme.borderColor.val,
                    },
                ]}
            >
                {options.map(option => (
                    <Pressable
                        key={option.value}
                        onPress={() => onSelect(option.value)}
                        style={[
                            styles.pickerMenuItem,
                            option.value === value && {
                                backgroundColor: theme.backgroundFocus.val,
                            },
                        ]}
                    >
                        <Text style={[styles.pickerMenuText, { color: theme.color.val }]}>
                            {option.label}
                        </Text>
                    </Pressable>
                ))}
            </View>
        </>
    )
}

const styles = StyleSheet.create({
    overlay: {
        position: 'absolute',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        zIndex: 99,
    },
    panel: {
        position: 'absolute',
        top: 4,
        left: 0,
        right: 0,
        zIndex: 100,
        borderRadius: 12,
        borderWidth: 1,
        maxHeight: 480,
    },
    scrollContent: {
        padding: 16,
    },
    fieldRow: {
        flexDirection: 'row',
        alignItems: 'center',
        marginBottom: 12,
        minHeight: 36,
    },
    fieldLabel: {
        width: 110,
        fontSize: 13,
        flexShrink: 0,
    },
    fieldValue: {
        flex: 1,
    },
    input: {
        flex: 1,
        fontSize: 14,
        borderBottomWidth: 1,
        paddingVertical: 4,
        paddingHorizontal: 2,
    },
    numberInput: {
        maxWidth: 80,
    },
    compoundRow: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 8,
    },
    checkboxRow: {
        flexDirection: 'row',
        alignItems: 'center',
        paddingVertical: 4,
    },
    footer: {
        flexDirection: 'row',
        justifyContent: 'flex-end',
        alignItems: 'center',
        gap: 12,
        paddingHorizontal: 16,
        paddingVertical: 12,
        borderTopWidth: StyleSheet.hairlineWidth,
    },
    clearButton: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 4,
        paddingHorizontal: 12,
        paddingVertical: 8,
    },
    clearButtonText: {
        fontSize: 14,
    },
    searchButton: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 6,
        paddingHorizontal: 20,
        paddingVertical: 8,
        borderRadius: 20,
    },
    searchButtonText: {
        fontSize: 14,
        fontWeight: '600',
    },
    pickerContainer: {
        position: 'relative',
    },
    pickerButton: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 4,
        borderBottomWidth: 1,
        paddingVertical: 4,
        paddingHorizontal: 2,
    },
    pickerText: {
        fontSize: 14,
    },
    pickerOverlay: {
        position: 'absolute',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        zIndex: 200,
    },
    pickerOverlayWeb: {
        // @ts-expect-error 'fixed' is valid on web but not in RN types
        position: 'fixed',
    },
    pickerMenu: {
        position: 'absolute',
        top: '100%',
        left: 0,
        zIndex: 201,
        minWidth: 140,
        borderRadius: 8,
        borderWidth: 1,
        paddingVertical: 4,
        marginTop: 2,
    },
    pickerMenuItem: {
        paddingHorizontal: 12,
        paddingVertical: 8,
    },
    pickerMenuText: {
        fontSize: 13,
    },
})
