import { useState } from 'react'
import { type Control, type FieldErrors, useController } from 'react-hook-form'
import { Pressable, StyleSheet, Text, TextInput, View } from 'react-native'
import { useTheme } from 'tamagui'
import type { ComposeFormData } from '../hooks/composeSchema'
import { RecipientField } from './RecipientField'

interface ComposeFieldsProps {
    control: Control<ComposeFormData>
    errors: FieldErrors<ComposeFormData>
}

function ComposeFieldInput({
    control,
    name,
}: {
    control: Control<ComposeFormData>
    name: keyof ComposeFormData
}) {
    const theme = useTheme()
    const { field } = useController({ control, name })

    return (
        <TextInput
            style={[styles.fieldInput, { color: theme.color.val }]}
            placeholderTextColor={theme.placeholderColor.val}
            value={field.value}
            onChangeText={field.onChange}
            onBlur={field.onBlur}
        />
    )
}

export function ComposeFields({ control, errors }: ComposeFieldsProps) {
    const theme = useTheme()
    const [showCc, setShowCc] = useState(false)
    const [showBcc, setShowBcc] = useState(false)

    const borderStyle = { borderBottomColor: theme.borderColor.val }

    return (
        <View style={styles.fieldsWrapper}>
            <View
                style={[
                    styles.recipientRow,
                    borderStyle,
                    errors.to ? { borderBottomColor: theme.red8.val } : undefined,
                ]}
            >
                <Text style={[styles.fieldLabel, { color: theme.color8.val }]}>To</Text>
                <RecipientField control={control} name="to" />
                {!showCc || !showBcc ? (
                    <View style={styles.ccBccButtons}>
                        {showCc ? null : (
                            <Pressable onPress={() => setShowCc(true)}>
                                <Text style={[styles.ccBccText, { color: theme.color8.val }]}>
                                    Cc
                                </Text>
                            </Pressable>
                        )}
                        {showBcc ? null : (
                            <Pressable onPress={() => setShowBcc(true)}>
                                <Text style={[styles.ccBccText, { color: theme.color8.val }]}>
                                    Bcc
                                </Text>
                            </Pressable>
                        )}
                    </View>
                ) : null}
            </View>
            {showCc ? (
                <View
                    style={[
                        styles.recipientRow,
                        borderStyle,
                        errors.cc ? { borderBottomColor: theme.red8.val } : undefined,
                    ]}
                >
                    <Text style={[styles.fieldLabel, { color: theme.color8.val }]}>Cc</Text>
                    <RecipientField control={control} name="cc" />
                </View>
            ) : null}
            {showBcc ? (
                <View
                    style={[
                        styles.recipientRow,
                        borderStyle,
                        errors.bcc ? { borderBottomColor: theme.red8.val } : undefined,
                    ]}
                >
                    <Text style={[styles.fieldLabel, { color: theme.color8.val }]}>Bcc</Text>
                    <RecipientField control={control} name="bcc" />
                </View>
            ) : null}
            <View
                style={[
                    styles.fieldRow,
                    borderStyle,
                    errors.subject ? { borderBottomColor: theme.red8.val } : undefined,
                ]}
            >
                <Text style={[styles.fieldLabel, { color: theme.color8.val }]}>Subject</Text>
                <ComposeFieldInput control={control} name="subject" />
            </View>
        </View>
    )
}

const styles = StyleSheet.create({
    fieldsWrapper: {
        zIndex: 10,
        overflow: 'visible' as 'visible',
    },
    recipientRow: {
        flexDirection: 'row',
        alignItems: 'center',
        paddingHorizontal: 12,
        minHeight: 36,
        paddingVertical: 4,
        borderBottomWidth: 1,
        zIndex: 11,
        overflow: 'visible' as 'visible',
    },
    fieldRow: {
        flexDirection: 'row',
        alignItems: 'center',
        paddingHorizontal: 12,
        height: 36,
        borderBottomWidth: 1,
    },
    fieldLabel: {
        fontSize: 13,
        width: 56,
    },
    fieldInput: {
        flex: 1,
        fontSize: 13,
        paddingHorizontal: 4,
        height: 28,
        outlineStyle: 'none',
    } as Record<string, unknown>,
    ccBccButtons: {
        flexDirection: 'row',
        gap: 8,
        marginLeft: 8,
    },
    ccBccText: {
        fontSize: 13,
        fontWeight: '500',
    },
})
