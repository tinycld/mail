import { useCallback, useRef } from 'react'
import { Platform } from 'react-native'

const DOUBLE_CLICK_MS = 400

export function useDoubleClick(onSingleClick: () => void, onDoubleClick: () => void) {
    const lastTapRef = useRef(0)

    return useCallback(() => {
        const now = Date.now()
        if (Platform.OS === 'web' && now - lastTapRef.current < DOUBLE_CLICK_MS) {
            onDoubleClick()
        } else {
            onSingleClick()
        }
        lastTapRef.current = now
    }, [onSingleClick, onDoubleClick])
}
