import { openHelp } from '@tinycld/core/lib/help/open-help'
import { appHref } from '@tinycld/core/lib/org-routes'
import { useRouter } from 'expo-router'
import { Pressable, Text } from 'react-native'
import { MAIL_APPS_PATH } from './mail-app-setup'

export function MailAppLink() {
    return (
        <Pressable onPress={() => openHelp('mail:imap')}>
            <Text className="text-primary text-xs font-semibold">Set up a mail app</Text>
        </Pressable>
    )
}

/** For the signed-in person: opens Settings → Account → Mail apps. */
export function MailAppSetupLink() {
    const router = useRouter()
    return (
        <Pressable onPress={() => router.push(appHref(MAIL_APPS_PATH))}>
            <Text className="text-primary text-xs font-semibold">Set up a mail app</Text>
        </Pressable>
    )
}
