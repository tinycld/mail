import { useThemeColor } from '@tinycld/core/lib/use-app-theme'
import {
    type Control,
    Controller,
    FormErrorSummary,
    useForm,
    zodResolver,
} from '@tinycld/core/ui/form'
import { PlainInput } from '@tinycld/core/ui/PlainInput'
import { ChevronDown, Search, Square, SquareCheck, X } from 'lucide-react-native'
import { useCallback, useMemo, useState } from 'react'
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
import type { AdvancedSearchFilters } from '../hooks/useSearchState'
import { type AdvancedSearchFormValues, advancedSearchSchema } from './advancedSearchSchema'

type FormValues = AdvancedSearchFormValues
type FormControl = Control<FormValues>

function makeDefaultValues(): FormValues {
    return {
        from: '',
        to: '',
        subject: '',
        hasWords: '',
        dateWithin: '',
        dateAnchor: new Date().toISOString().split('T')[0],
        folder: 'all',
        hasAttachment: false,
    }
}

function formDataToFilters(data: FormValues): AdvancedSearchFilters {
    const filters: AdvancedSearchFilters = {}
    const textFields = ['from', 'to', 'subject', 'hasWords'] as const
    for (const key of textFields) {
        if (data[key]) filters[key] = data[key]
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
    const foregroundColor = useThemeColor('foreground')
    const mutedColor = useThemeColor('muted-foreground')
    const borderColor = useThemeColor('border')
    const backgroundColor = useThemeColor('background')
    const primaryColor = useThemeColor('primary')
    const primaryFgColor = useThemeColor('primary-foreground')
    const shadowColor = useThemeColor('border')
    const focusBgColor = useThemeColor('surface-secondary')

    const defaults = useMemo(
        () => ({ ...makeDefaultValues(), ...initialFilters }),
        [initialFilters]
    )

    const {
        control,
        handleSubmit,
        reset,
        formState: { errors, isSubmitted },
    } = useForm({
        resolver: zodResolver(advancedSearchSchema),
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

    const inputStyle = [
        styles.input,
        {
            color: foregroundColor,
            borderColor,
        },
    ]

    return (
        <>
            <Pressable style={styles.overlay} onPress={onClose} />
            <View
                testID="advanced-search-panel"
                style={[
                    {
                        position: 'absolute',
                        top: 4,
                        left: 0,
                        right: 0,
                        zIndex: 100,
                        borderRadius: 12,
                        borderWidth: 1,
                        maxHeight: 480,
                        // Clip children to the rounded border on narrow (iOS)
                        // screens so picker rows/menus don't bleed past the
                        // right edge.
                        overflow: 'hidden',
                        backgroundColor,
                        borderColor,
                    },
                    Platform.select({
                        web: {
                            boxShadow: `0 4px 24px ${shadowColor}`,
                        },
                        default: {
                            shadowColor,
                            shadowOffset: { width: 0, height: 4 },
                            shadowOpacity: 0.15,
                            shadowRadius: 12,
                            elevation: 8,
                        },
                    }),
                ]}
            >
                <ScrollView style={{ padding: 16 }} keyboardShouldPersistTaps="handled">
                    <FormErrorSummary
                        errors={errors}
                        isEnabled={isSubmitted}
                        testID="advanced-search-errors"
                    />
                    <FieldRow label="From" labelColor={mutedColor}>
                        <FormInput
                            control={control}
                            name="from"
                            style={inputStyle}
                            placeholder=""
                        />
                    </FieldRow>
                    <FieldRow label="To" labelColor={mutedColor}>
                        <FormInput control={control} name="to" style={inputStyle} placeholder="" />
                    </FieldRow>
                    <FieldRow label="Subject" labelColor={mutedColor}>
                        <FormInput
                            control={control}
                            name="subject"
                            style={inputStyle}
                            placeholder=""
                        />
                    </FieldRow>
                    <FieldRow label="Body" labelColor={mutedColor}>
                        <FormInput
                            control={control}
                            name="hasWords"
                            style={inputStyle}
                            placeholder=""
                        />
                    </FieldRow>
                    <FieldRow label="Date within" labelColor={mutedColor}>
                        <View className="flex-row items-center gap-2">
                            <PickerButton
                                control={control}
                                name="dateWithin"
                                options={DATE_WITHIN_OPTIONS}
                                foregroundColor={foregroundColor}
                                mutedColor={mutedColor}
                                borderColor={borderColor}
                                backgroundColor={backgroundColor}
                                focusBgColor={focusBgColor}
                            />
                            <FormInput
                                control={control}
                                name="dateAnchor"
                                style={inputStyle}
                                placeholder="YYYY-MM-DD"
                            />
                        </View>
                    </FieldRow>
                    <FieldRow label="Search" labelColor={mutedColor}>
                        <PickerButton
                            control={control}
                            name="folder"
                            options={FOLDER_OPTIONS}
                            foregroundColor={foregroundColor}
                            mutedColor={mutedColor}
                            borderColor={borderColor}
                            backgroundColor={backgroundColor}
                            focusBgColor={focusBgColor}
                        />
                    </FieldRow>
                    <FieldRow label="Has attachment" labelColor={mutedColor}>
                        <AttachmentCheckbox
                            control={control}
                            checkedColor={primaryColor}
                            uncheckedColor={mutedColor}
                        />
                    </FieldRow>
                </ScrollView>
                <View
                    className="flex-row justify-end items-center gap-3 px-4 py-3"
                    style={{
                        borderTopWidth: StyleSheet.hairlineWidth,
                        borderTopColor: borderColor,
                    }}
                >
                    <Pressable onPress={onClear} className="flex-row items-center gap-1 px-3 py-2">
                        <X size={14} color={mutedColor} />
                        <Text style={{ fontSize: 14, color: mutedColor }}>Clear</Text>
                    </Pressable>
                    <Pressable
                        testID="advanced-search-submit"
                        onPress={onSubmit}
                        className="flex-row items-center py-2 rounded-full"
                        style={{
                            gap: 6,
                            paddingHorizontal: 20,
                            backgroundColor: primaryColor,
                        }}
                    >
                        <Search size={16} color={primaryFgColor} />
                        <Text style={{ fontSize: 14, fontWeight: '600', color: primaryFgColor }}>
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
        <View className="flex-row items-center mb-3" style={{ minHeight: 36 }}>
            <Text style={{ width: 96, fontSize: 13, flexShrink: 0, color: labelColor }}>
                {label}
            </Text>
            <View className="flex-1" style={{ minWidth: 0 }}>
                {children}
            </View>
        </View>
    )
}

function AttachmentCheckbox({
    control,
    checkedColor,
    uncheckedColor,
}: {
    control: FormControl
    checkedColor: string
    uncheckedColor: string
}) {
    return (
        <Controller
            control={control}
            name="hasAttachment"
            render={({ field: { value, onChange } }) => (
                <Pressable onPress={() => onChange(!value)} className="flex-row items-center py-1">
                    {value ? (
                        <SquareCheck size={20} color={checkedColor} />
                    ) : (
                        <Square size={20} color={uncheckedColor} />
                    )}
                </Pressable>
            )}
        />
    )
}

type TextFieldName = 'from' | 'to' | 'subject' | 'hasWords' | 'dateAnchor'

function FormInput({
    control,
    name,
    style,
    placeholder,
    keyboardType,
}: {
    control: FormControl
    name: TextFieldName
    style: StyleProp<TextStyle>
    placeholder: string
    keyboardType?: 'numeric' | 'default'
}) {
    return (
        <Controller
            control={control}
            name={name}
            render={({ field: { value, onChange } }) => (
                <PlainInput
                    testID={`advanced-search-input-${name}`}
                    style={style}
                    value={value}
                    onChangeText={onChange}
                    placeholder={placeholder}
                    keyboardType={keyboardType}
                />
            )}
        />
    )
}

interface PickerOption {
    label: string
    value: string
}

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
    foregroundColor,
    mutedColor,
    borderColor,
    backgroundColor,
    focusBgColor,
}: {
    control: FormControl
    name: 'dateWithin' | 'folder'
    options: PickerOption[]
    foregroundColor: string
    mutedColor: string
    borderColor: string
    backgroundColor: string
    focusBgColor: string
}) {
    return (
        <Controller
            control={control}
            name={name}
            render={({ field: { value, onChange } }) => (
                <PickerField
                    options={options}
                    value={value}
                    onChange={onChange}
                    foregroundColor={foregroundColor}
                    mutedColor={mutedColor}
                    borderColor={borderColor}
                    backgroundColor={backgroundColor}
                    focusBgColor={focusBgColor}
                />
            )}
        />
    )
}

function PickerField({
    options,
    value,
    onChange,
    foregroundColor,
    mutedColor,
    borderColor,
    backgroundColor,
    focusBgColor,
}: {
    options: PickerOption[]
    value: string
    onChange: (value: string) => void
    foregroundColor: string
    mutedColor: string
    borderColor: string
    backgroundColor: string
    focusBgColor: string
}) {
    const [isOpen, setIsOpen] = useState(false)
    const selectedLabel = options.find(o => o.value === value)?.label ?? options[0].label

    return (
        <View className="relative">
            <Pressable
                onPress={() => setIsOpen(!isOpen)}
                className="flex-row items-center gap-1 py-1"
                style={{
                    borderBottomWidth: 1,
                    paddingHorizontal: 2,
                    borderBottomColor: borderColor,
                }}
            >
                <Text style={{ fontSize: 14, color: foregroundColor }} numberOfLines={1}>
                    {selectedLabel}
                </Text>
                <ChevronDown size={14} color={mutedColor} />
            </Pressable>
            {isOpen ? (
                <PickerMenu
                    options={options}
                    value={value}
                    onSelect={val => {
                        onChange(val)
                        setIsOpen(false)
                    }}
                    onClose={() => setIsOpen(false)}
                    foregroundColor={foregroundColor}
                    borderColor={borderColor}
                    backgroundColor={backgroundColor}
                    focusBgColor={focusBgColor}
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
    foregroundColor,
    borderColor,
    backgroundColor,
    focusBgColor,
}: {
    options: PickerOption[]
    value: string
    onSelect: (value: string) => void
    onClose: () => void
    foregroundColor: string
    borderColor: string
    backgroundColor: string
    focusBgColor: string
}) {
    return (
        <>
            <Pressable
                style={[styles.pickerOverlay, Platform.OS === 'web' && styles.pickerOverlayWeb]}
                onPress={onClose}
            />
            <View
                className="absolute border rounded-lg py-1"
                style={{
                    top: '100%',
                    left: 0,
                    zIndex: 201,
                    minWidth: 140,
                    maxWidth: 200,
                    marginTop: 2,
                    backgroundColor,
                    borderColor,
                }}
            >
                {options.map(option => (
                    <Pressable
                        key={option.value}
                        onPress={() => onSelect(option.value)}
                        className="px-3 py-2"
                        style={[
                            option.value === value && {
                                backgroundColor: focusBgColor,
                            },
                        ]}
                    >
                        <Text style={{ fontSize: 13, color: foregroundColor }}>{option.label}</Text>
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
    input: {
        flex: 1,
        fontSize: 14,
        borderBottomWidth: 1,
        paddingVertical: 4,
        paddingHorizontal: 2,
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
        position: 'fixed' as 'absolute',
    },
})
