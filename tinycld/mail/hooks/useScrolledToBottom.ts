import { useCallback, useRef, useState } from 'react'
import type { NativeScrollEvent, NativeSyntheticEvent } from 'react-native'

const BOTTOM_THRESHOLD_PX = 24

export function isScrolledToBottom(metrics: {
    contentOffsetY: number
    contentHeight: number
    layoutHeight: number
}) {
    const distanceFromBottom = metrics.contentHeight - (metrics.contentOffsetY + metrics.layoutHeight)
    return distanceFromBottom <= BOTTOM_THRESHOLD_PX
}

export function useScrolledToBottom() {
    const [isAtBottom, setIsAtBottom] = useState(false)
    const wasAtBottom = useRef(false)

    const onScroll = useCallback((e: NativeSyntheticEvent<NativeScrollEvent>) => {
        const { contentOffset, contentSize, layoutMeasurement } = e.nativeEvent
        const atBottom = isScrolledToBottom({
            contentOffsetY: contentOffset.y,
            contentHeight: contentSize.height,
            layoutHeight: layoutMeasurement.height,
        })
        if (atBottom !== wasAtBottom.current) {
            wasAtBottom.current = atBottom
            setIsAtBottom(atBottom)
        }
    }, [])

    return { isAtBottom, onScroll }
}
