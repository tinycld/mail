import { openHelp } from '@tinycld/core/lib/help/open-help'
import { Pressable, Text } from 'react-native'

export function MailAppLink() {
    return (
        <Pressable onPress={() => openHelp('mail:imap')}>
            <Text className="text-primary text-xs font-semibold">Set up a mail app</Text>
        </Pressable>
    )
}
