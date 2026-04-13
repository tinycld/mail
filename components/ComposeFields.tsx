import { useState } from 'react'
import { type Control, type FieldErrors, useController } from 'react-hook-form'
import { Pressable, Text, View } from 'react-native'
import { useThemeColor } from '~/lib/use-app-theme'
import { PlainInput } from '~/ui/PlainInput'
import type { ComposeFormData } from '../hooks/composeSchema'
import { RecipientField } from './RecipientField'

interface ComposeFieldsProps {
    control: Control<ComposeFormData>
    errors: FieldErrors<ComposeFormData>
    onSubjectBlur?: () => void
}

function ComposeFieldInput({
    control,
    name,
    onBlur: onBlurExtra,
}: {
    control: Control<ComposeFormData>
    name: keyof ComposeFormData
    onBlur?: () => void
}) {
    const foregroundColor = useThemeColor('foreground')
    const placeholderColor = useThemeColor('field-placeholder')
    const { field } = useController({ control, name })

    return (
        <PlainInput
            style={{ flex: 1, fontSize: 13, paddingHorizontal: 4, color: foregroundColor }}
            placeholderTextColor={placeholderColor}
            value={field.value}
            onChangeText={field.onChange}
            onBlur={() => {
                field.onBlur()
                onBlurExtra?.()
            }}
        />
    )
}

export function ComposeFields({ control, errors, onSubjectBlur }: ComposeFieldsProps) {
    const mutedColor = useThemeColor('muted-foreground')
    const borderColor = useThemeColor('border')
    const dangerColor = useThemeColor('danger')
    const [showCc, setShowCc] = useState(false)
    const [showBcc, setShowBcc] = useState(false)

    return (
        <View style={{ zIndex: 10, overflow: 'visible' }}>
            <View
                style={{
                    flexDirection: 'row',
                    alignItems: 'center',
                    paddingHorizontal: 12,
                    minHeight: 36,
                    paddingVertical: 4,
                    borderBottomWidth: 1,
                    zIndex: 11,
                    overflow: 'visible',
                    borderBottomColor: errors.to ? dangerColor : borderColor,
                }}
            >
                <Text style={{ fontSize: 13, width: 56, color: mutedColor }}>To</Text>
                <RecipientField control={control} name="to" />
                {!showCc || !showBcc ? (
                    <View style={{ flexDirection: 'row', gap: 8, marginLeft: 8 }}>
                        {showCc ? null : (
                            <Pressable onPress={() => setShowCc(true)}>
                                <Text
                                    style={{ fontSize: 13, fontWeight: '500', color: mutedColor }}
                                >
                                    Cc
                                </Text>
                            </Pressable>
                        )}
                        {showBcc ? null : (
                            <Pressable onPress={() => setShowBcc(true)}>
                                <Text
                                    style={{ fontSize: 13, fontWeight: '500', color: mutedColor }}
                                >
                                    Bcc
                                </Text>
                            </Pressable>
                        )}
                    </View>
                ) : null}
            </View>
            {showCc ? (
                <View
                    style={{
                        flexDirection: 'row',
                        alignItems: 'center',
                        paddingHorizontal: 12,
                        minHeight: 36,
                        paddingVertical: 4,
                        borderBottomWidth: 1,
                        zIndex: 11,
                        overflow: 'visible',
                        borderBottomColor: errors.cc ? dangerColor : borderColor,
                    }}
                >
                    <Text style={{ fontSize: 13, width: 56, color: mutedColor }}>Cc</Text>
                    <RecipientField control={control} name="cc" />
                </View>
            ) : null}
            {showBcc ? (
                <View
                    style={{
                        flexDirection: 'row',
                        alignItems: 'center',
                        paddingHorizontal: 12,
                        minHeight: 36,
                        paddingVertical: 4,
                        borderBottomWidth: 1,
                        zIndex: 11,
                        overflow: 'visible',
                        borderBottomColor: errors.bcc ? dangerColor : borderColor,
                    }}
                >
                    <Text style={{ fontSize: 13, width: 56, color: mutedColor }}>Bcc</Text>
                    <RecipientField control={control} name="bcc" />
                </View>
            ) : null}
            <View
                style={{
                    flexDirection: 'row',
                    alignItems: 'center',
                    paddingHorizontal: 12,
                    height: 36,
                    borderBottomWidth: 1,
                    borderBottomColor: errors.subject ? dangerColor : borderColor,
                }}
            >
                <Text style={{ fontSize: 13, width: 56, color: mutedColor }}>Subject</Text>
                <ComposeFieldInput control={control} name="subject" onBlur={onSubjectBlur} />
            </View>
        </View>
    )
}
