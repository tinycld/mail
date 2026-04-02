import type { Control, FieldErrors } from 'react-hook-form'
import type { z } from 'zod'
import { User, Building2, Mail, Phone, FileText } from 'lucide-react-native'
import { XStack, YStack } from 'tamagui'
import { TextInput, TextAreaInput, Toggle, FormErrorSummary } from '~/ui/form'
import type { contactSchema } from './contactSchema'

type ContactFormValues = z.infer<typeof contactSchema>

interface ContactFormProps {
    control: Control<ContactFormValues>
    errors: FieldErrors<ContactFormValues>
    isSubmitted: boolean
}

export function ContactForm({ control, errors, isSubmitted }: ContactFormProps) {
    return (
        <YStack gap="$1">
            <FormErrorSummary errors={errors} isEnabled={isSubmitted} />

            <XStack gap="$2">
                <YStack flex={1}>
                    <TextInput
                        name="first_name"
                        control={control}
                        label="First name"
                        labelIcon={User}
                        placeholder="First name"
                        autoFocus
                    />
                </YStack>
                <YStack flex={1}>
                    <TextInput
                        name="last_name"
                        control={control}
                        label="Last name"
                        placeholder="Last name"
                    />
                </YStack>
            </XStack>

            <TextInput
                name="company"
                control={control}
                label="Company"
                labelIcon={Building2}
                placeholder="Company"
            />

            <TextInput
                name="job_title"
                control={control}
                label="Job title"
                placeholder="Job title"
            />

            <TextInput
                name="email"
                control={control}
                label="Email"
                labelIcon={Mail}
                placeholder="email@example.com"
                keyboardType="email-address"
                autoCapitalize="none"
            />

            <TextInput
                name="phone"
                control={control}
                label="Phone"
                labelIcon={Phone}
                placeholder="Phone number"
                keyboardType="phone-pad"
            />

            <TextAreaInput
                name="notes"
                control={control}
                label="Notes"
                labelIcon={FileText}
                placeholder="Optional notes..."
                numberOfLines={3}
            />

            <Toggle
                name="favorite"
                control={control}
                label="Favorite"
            />
        </YStack>
    )
}
