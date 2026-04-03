import { useState } from 'react'
import { ActivityIndicator, Pressable, StyleSheet, Text, TextInput, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useAuth } from '~/lib/auth'
import { pb } from '~/lib/pocketbase'

interface SignupModalProps {
    onSwitchToLogin: () => void
}

export function SignupModal({ onSwitchToLogin }: SignupModalProps) {
    const theme = useTheme()
    const { login } = useAuth({ throwIfAnon: false })
    const [orgName, setOrgName] = useState('')
    const [orgSlug, setOrgSlug] = useState('')
    const [email, setEmail] = useState('')
    const [password, setPassword] = useState('')
    const [error, setError] = useState<string | null>(null)
    const [isSubmitting, setIsSubmitting] = useState(false)

    const canSubmit =
        orgName.trim().length > 0 &&
        orgSlug.trim().length > 0 &&
        email.trim().length > 0 &&
        password.length >= 8 &&
        !isSubmitting

    const deriveSlug = (name: string) => {
        return name
            .toLowerCase()
            .replace(/[^a-z0-9]+/g, '-')
            .replace(/^-|-$/g, '')
            .slice(0, 15)
    }

    const handleOrgNameChange = (value: string) => {
        setOrgName(value)
        if (!orgSlug || orgSlug === deriveSlug(orgName)) {
            setOrgSlug(deriveSlug(value))
        }
    }

    const handleSubmit = async () => {
        if (!canSubmit) return
        setError(null)
        setIsSubmitting(true)

        try {
            const response = await pb.send('/api/signup', {
                method: 'POST',
                body: {
                    email: email.trim(),
                    password,
                    orgName: orgName.trim(),
                    orgSlug: orgSlug.trim(),
                },
            })

            if (!response.userId) {
                setError('Signup failed unexpectedly')
                setIsSubmitting(false)
                return
            }

            const result = await login(email.trim(), password)
            if (result.error) {
                setError(result.error)
                setIsSubmitting(false)
            }
        } catch (err) {
            const message = err instanceof Error ? err.message : 'Failed to create account'
            setError(message)
            setIsSubmitting(false)
        }
    }

    return (
        <View style={[styles.backdrop, { backgroundColor: theme.modalOverlay.val }]}>
            <View
                style={[
                    styles.modal,
                    { backgroundColor: theme.background.val, borderColor: theme.borderColor.val },
                ]}
            >
                <Text style={[styles.title, { color: theme.color.val }]}>Create account</Text>
                <Text style={[styles.subtitle, { color: theme.color8.val }]}>
                    Set up your organization and account
                </Text>

                {error && (
                    <View style={[styles.errorContainer, { backgroundColor: theme.red2.val }]}>
                        <Text style={{ color: theme.red8.val, fontSize: 14 }}>{error}</Text>
                    </View>
                )}

                <View style={styles.fieldGroup}>
                    <Text style={[styles.label, { color: theme.color.val }]}>
                        Organization name
                    </Text>
                    <TextInput
                        style={[styles.input, inputColors(theme)]}
                        value={orgName}
                        onChangeText={handleOrgNameChange}
                        placeholder="Acme Corp"
                        placeholderTextColor={theme.color8.val}
                        autoCapitalize="words"
                        editable={!isSubmitting}
                    />
                </View>

                <View style={styles.fieldGroup}>
                    <Text style={[styles.label, { color: theme.color.val }]}>
                        Organization slug
                    </Text>
                    <TextInput
                        style={[styles.input, inputColors(theme)]}
                        value={orgSlug}
                        onChangeText={setOrgSlug}
                        placeholder="acme-corp"
                        placeholderTextColor={theme.color8.val}
                        autoCapitalize="none"
                        autoCorrect={false}
                        editable={!isSubmitting}
                    />
                    <Text style={[styles.hint, { color: theme.color8.val }]}>
                        3-15 chars, lowercase letters, numbers, hyphens
                    </Text>
                </View>

                <View style={styles.fieldGroup}>
                    <Text style={[styles.label, { color: theme.color.val }]}>Email</Text>
                    <TextInput
                        style={[styles.input, inputColors(theme)]}
                        value={email}
                        onChangeText={setEmail}
                        placeholder="you@example.com"
                        placeholderTextColor={theme.color8.val}
                        keyboardType="email-address"
                        autoCapitalize="none"
                        autoComplete="email"
                        editable={!isSubmitting}
                    />
                </View>

                <View style={styles.fieldGroup}>
                    <Text style={[styles.label, { color: theme.color.val }]}>Password</Text>
                    <TextInput
                        style={[styles.input, inputColors(theme)]}
                        value={password}
                        onChangeText={setPassword}
                        placeholder="At least 8 characters"
                        placeholderTextColor={theme.color8.val}
                        secureTextEntry
                        autoComplete="new-password"
                        editable={!isSubmitting}
                        onSubmitEditing={handleSubmit}
                    />
                </View>

                <Pressable
                    style={[
                        styles.submitButton,
                        { backgroundColor: theme.accentBackground.val },
                        !canSubmit && styles.submitButtonDisabled,
                    ]}
                    onPress={handleSubmit}
                    disabled={!canSubmit}
                >
                    {isSubmitting ? (
                        <ActivityIndicator color={theme.accentColor.val} size="small" />
                    ) : (
                        <Text style={[styles.submitButtonText, { color: theme.accentColor.val }]}>
                            Create account
                        </Text>
                    )}
                </Pressable>

                <Pressable style={styles.switchLink} onPress={onSwitchToLogin}>
                    <Text style={{ color: theme.color8.val, fontSize: 14 }}>
                        Already have an account?{' '}
                        <Text style={{ color: theme.accentBackground.val, fontWeight: '600' }}>
                            Sign in
                        </Text>
                    </Text>
                </Pressable>
            </View>
        </View>
    )
}

function inputColors(theme: ReturnType<typeof useTheme>) {
    return {
        color: theme.color.val,
        borderColor: theme.borderColor.val,
        backgroundColor: theme.backgroundHover.val,
    }
}

const styles = StyleSheet.create({
    backdrop: {
        position: 'absolute',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        justifyContent: 'center',
        alignItems: 'center',
        zIndex: 200,
    },
    modal: {
        width: 400,
        maxWidth: '90%',
        borderRadius: 16,
        borderWidth: 1,
        padding: 32,
        shadowColor: '#000',
        shadowOffset: { width: 0, height: 8 },
        shadowOpacity: 0.15,
        shadowRadius: 24,
        elevation: 8,
    },
    title: {
        fontSize: 22,
        fontWeight: '700',
        marginBottom: 4,
    },
    subtitle: {
        fontSize: 14,
        marginBottom: 24,
    },
    errorContainer: {
        borderRadius: 8,
        padding: 12,
        marginBottom: 16,
    },
    fieldGroup: {
        marginBottom: 16,
    },
    label: {
        fontSize: 14,
        fontWeight: '600',
        marginBottom: 6,
    },
    hint: {
        fontSize: 12,
        marginTop: 4,
    },
    input: {
        borderWidth: 1,
        borderRadius: 8,
        padding: 12,
        fontSize: 16,
    },
    submitButton: {
        borderRadius: 8,
        padding: 14,
        alignItems: 'center',
        marginTop: 8,
    },
    submitButtonDisabled: {
        opacity: 0.5,
    },
    submitButtonText: {
        fontSize: 16,
        fontWeight: '600',
    },
    switchLink: {
        alignItems: 'center',
        marginTop: 16,
    },
})
