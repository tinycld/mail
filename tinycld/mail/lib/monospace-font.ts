import { Platform } from 'react-native'

export const MONOSPACE_FONT = Platform.select({
    ios: 'Menlo',
    android: 'monospace',
    default: 'monospace',
})
