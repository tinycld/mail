import { QrCode } from '@tinycld/core/components/settings/security/QrCode'
import { useFileToken } from '@tinycld/core/file-viewer/use-authed-file-url'
import { openHelp } from '@tinycld/core/lib/help/open-help'
import { log } from '@tinycld/core/lib/logger'
import { getResolvedAddress } from '@tinycld/core/lib/server-address'
import { useCurrentRole } from '@tinycld/core/lib/use-current-role'
import { useMailHost } from '@tinycld/core/lib/use-mail-host'
import { Button, ButtonText } from '@tinycld/core/ui/button'
import { Linking, Pressable, ScrollView, Text, View } from 'react-native'
import { CopyButton } from './CopyButton'
import { IMAP_PORT, profileUrl, SMTP_PORT, setupAddress, setupPageUrl } from './mail-app-setup'
import { useMyPersonEmail } from './usePersonEmail'

const IMAP_SECURITY = `Port ${IMAP_PORT}, SSL/TLS`
const SMTP_SECURITY = `Port ${SMTP_PORT}, SSL/TLS`

function openUrl(url: string) {
    Linking.openURL(url).catch(err => log.error('mail.client-setup', err, { url: 'profile' }))
}

function useMailAppSetup() {
    const { identity, addresses, isReady } = useMyPersonEmail()
    const host = useMailHost()
    const { data: fileToken } = useFileToken()
    const serverAddress = getResolvedAddress()
    const address =
        identity && !identity.retired ? setupAddress(identity.defaultFrom, addresses) : null
    const profileHref = serverAddress && fileToken ? profileUrl(serverAddress, fileToken) : null
    return {
        address,
        host,
        hasNoAddress: isReady && address === null,
        pageUrl: serverAddress ? setupPageUrl(serverAddress) : null,
        openProfile: profileHref ? () => openUrl(profileHref) : null,
    }
}

function SettingRow({ label, value, testID }: { label: string; value: string; testID: string }) {
    return (
        <View className="gap-1">
            <Text className="text-muted-foreground text-xs">{label}</Text>
            <View className="flex-row items-center gap-2">
                <Text testID={testID} className="flex-1 text-foreground text-sm" selectable>
                    {value}
                </Text>
                <CopyButton value={value} label={`Copy ${label}`} />
            </View>
        </View>
    )
}

function Note({ isVisible, text }: { isVisible: boolean; text: string }) {
    if (!isVisible) return null
    return <Text className="text-muted-foreground text-sm">{text}</Text>
}

function ServerSettings({ address, host }: { address: string; host: string }) {
    return (
        <View className="gap-3 rounded-xl border border-border p-3">
            <SettingRow label="Email address" value={address} testID="mail-app-address" />
            <SettingRow
                label="Incoming mail server (IMAP)"
                value={host}
                testID="mail-app-imap-host"
            />
            <Text className="text-muted-foreground text-xs">{IMAP_SECURITY}</Text>
            <SettingRow
                label="Outgoing mail server (SMTP)"
                value={host}
                testID="mail-app-smtp-host"
            />
            <Text className="text-muted-foreground text-xs">{SMTP_SECURITY}</Text>
            <SettingRow label="User name" value={address} testID="mail-app-user-name" />
            <Text className="text-muted-foreground text-xs">
                Password: the password you sign in with.
            </Text>
        </View>
    )
}

function AppleProfile({ onPress }: { onPress: (() => void) | null }) {
    return (
        <View className="gap-2">
            <Text className="text-sm font-semibold text-foreground">Apple Mail</Text>
            <Text className="text-xs text-muted-foreground">
                On a Mac, iPhone or iPad, the profile puts these settings in Mail. The profile does
                not hold your password: your device asks for it. After you download it, open it to
                install it. On an iPhone or iPad, open Settings and tap Profile Downloaded. On a
                Mac, open System Settings, Privacy & Security, then Profiles. The device says the
                profile is Unverified. This is normal.
            </Text>
            <Button
                size="sm"
                testID="mail-app-profile-button"
                isDisabled={onPress === null}
                onPress={onPress ?? undefined}
            >
                <ButtonText>Download the profile</ButtonText>
            </Button>
        </View>
    )
}

function PhoneQr({ pageUrl }: { pageUrl: string | null }) {
    if (!pageUrl) return null
    return (
        <View className="gap-2">
            <Text className="text-sm font-semibold text-foreground">Your phone</Text>
            <Text className="text-xs text-muted-foreground">
                Scan this code with the camera of your phone. The code opens this page on your
                phone.
            </Text>
            <QrCode
                value={pageUrl}
                accessibilityLabel="QR code that opens this page on your phone"
            />
        </View>
    )
}

function SetupDetails({
    isVisible,
    address,
    host,
    pageUrl,
    openProfile,
}: {
    isVisible: boolean
    address: string | null
    host: string | null
    pageUrl: string | null
    openProfile: (() => void) | null
}) {
    if (!isVisible || !address || !host) return null
    return (
        <View className="gap-4">
            <Text className="text-sm text-foreground">
                Use these settings in Apple Mail, Outlook, Thunderbird or any other mail app. Most
                apps find them when you type your address and password.
            </Text>
            <ServerSettings address={address} host={host} />
            <AppleProfile onPress={openProfile} />
            <PhoneQr pageUrl={pageUrl} />
            <Pressable onPress={() => openHelp('mail:imap')}>
                <Text className="text-primary text-xs font-semibold">More help</Text>
            </Pressable>
        </View>
    )
}

/** Settings → Account → Mail apps: the person's server settings, an Apple profile and a QR code. */
export default function MailAppSetup() {
    const { address, host, hasNoAddress, pageUrl, openProfile } = useMailAppSetup()
    const role = useCurrentRole()
    return (
        <ScrollView contentContainerClassName="gap-4 p-4">
            <Note
                isVisible={hasNoAddress && role.isGuest}
                text="Guests do not get email addresses."
            />
            <Note isVisible={hasNoAddress && !role.isGuest} text="You have no email address yet." />
            <SetupDetails
                isVisible={!hasNoAddress}
                address={address}
                host={host}
                pageUrl={pageUrl}
                openProfile={openProfile}
            />
        </ScrollView>
    )
}
