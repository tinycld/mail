import { useCallback, useMemo, useRef, useState } from 'react'

const BUFFER_PX = 200

interface UseVirtualGridOptions {
    numRows: number
    numCols: number
    getColWidth: (col: number) => number
    getRowHeight: (row: number) => number
}

interface VirtualGridResult {
    visibleRows: [number, number]
    visibleCols: [number, number]
    colOffsets: number[]
    rowOffsets: number[]
    totalWidth: number
    totalHeight: number
    scrollX: number
    scrollY: number
    onLayout: (e: { nativeEvent: { layout: { width: number; height: number } } }) => void
    handleScrollX: (x: number) => void
    handleScrollY: (y: number) => void
}

function binarySearch(offsets: number[], target: number): number {
    let lo = 0
    let hi = offsets.length - 1
    while (lo <= hi) {
        const mid = (lo + hi) >>> 1
        if (offsets[mid] < target) {
            lo = mid + 1
        } else {
            hi = mid - 1
        }
    }
    return Math.max(0, lo - 1)
}

export function useVirtualGrid({
    numRows,
    numCols,
    getColWidth,
    getRowHeight,
}: UseVirtualGridOptions): VirtualGridResult {
    const [viewport, setViewport] = useState({ width: 0, height: 0 })
    const [scrollX, setScrollX] = useState(0)
    const [scrollY, setScrollY] = useState(0)
    const scrollXRef = useRef(0)
    const scrollYRef = useRef(0)

    const colOffsets = useMemo(() => {
        const offsets: number[] = new Array(numCols)
        let x = 0
        for (let c = 0; c < numCols; c++) {
            offsets[c] = x
            x += getColWidth(c)
        }
        return offsets
    }, [numCols, getColWidth])

    const rowOffsets = useMemo(() => {
        const offsets: number[] = new Array(numRows)
        let y = 0
        for (let r = 0; r < numRows; r++) {
            offsets[r] = y
            y += getRowHeight(r)
        }
        return offsets
    }, [numRows, getRowHeight])

    const totalWidth = numCols > 0 ? colOffsets[numCols - 1] + getColWidth(numCols - 1) : 0
    const totalHeight = numRows > 0 ? rowOffsets[numRows - 1] + getRowHeight(numRows - 1) : 0

    const visibleRows = useMemo((): [number, number] => {
        if (viewport.height === 0 || numRows === 0) return [0, 0]
        const first = binarySearch(rowOffsets, scrollY - BUFFER_PX)
        const last = Math.min(
            binarySearch(rowOffsets, scrollY + viewport.height + BUFFER_PX),
            numRows - 1
        )
        return [first, last]
    }, [rowOffsets, scrollY, viewport.height, numRows])

    const visibleCols = useMemo((): [number, number] => {
        if (viewport.width === 0 || numCols === 0) return [0, 0]
        const first = binarySearch(colOffsets, scrollX - BUFFER_PX)
        const last = Math.min(
            binarySearch(colOffsets, scrollX + viewport.width + BUFFER_PX),
            numCols - 1
        )
        return [first, last]
    }, [colOffsets, scrollX, viewport.width, numCols])

    const onLayout = useCallback(
        (e: { nativeEvent: { layout: { width: number; height: number } } }) => {
            const { width, height } = e.nativeEvent.layout
            setViewport(prev => {
                if (prev.width === width && prev.height === height) return prev
                return { width, height }
            })
        },
        []
    )

    const handleScrollX = useCallback((x: number) => {
        if (Math.abs(x - scrollXRef.current) > 1) {
            scrollXRef.current = x
            setScrollX(x)
        }
    }, [])

    const handleScrollY = useCallback((y: number) => {
        if (Math.abs(y - scrollYRef.current) > 1) {
            scrollYRef.current = y
            setScrollY(y)
        }
    }, [])

    return {
        visibleRows,
        visibleCols,
        colOffsets,
        rowOffsets,
        totalWidth,
        totalHeight,
        scrollX,
        scrollY,
        onLayout,
        handleScrollX,
        handleScrollY,
    }
}

export function cellFromPoint(
    x: number,
    y: number,
    colOffsets: number[],
    rowOffsets: number[]
): { row: number; col: number } {
    return {
        row: binarySearch(rowOffsets, y),
        col: binarySearch(colOffsets, x),
    }
}
