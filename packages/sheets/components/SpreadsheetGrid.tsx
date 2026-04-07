import { useCallback, useMemo, useRef } from 'react'
import { Platform, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useSpreadsheet } from '../hooks/useSpreadsheet'
import { cellFromPoint, useVirtualGrid } from '../hooks/useVirtualGrid'
import { colIndexToLetter, HEADER_HEIGHT, ROW_HEADER_WIDTH } from '../lib/cell-utils'
import { CellEditor } from './CellEditor'
import { CellRenderer } from './CellRenderer'

const isWeb = Platform.OS === 'web'

function handleUndoRedoKey(e: React.KeyboardEvent, undo: () => void, redo: () => void): boolean {
    const hasModifier = e.ctrlKey || e.metaKey
    if (!hasModifier) return false
    if (e.key === 'z' || e.key === 'y') {
        e.preventDefault()
        if (e.key === 'y' || e.shiftKey) redo()
        else undo()
        return true
    }
    return false
}

function handleGridKey(
    e: React.KeyboardEvent,
    selection: { row: number; col: number },
    gridDimensions: { rows: number; cols: number },
    isReadOnly: boolean,
    setSelection: (s: { row: number; col: number }) => void,
    startEditing: (s: { row: number; col: number }) => void,
    setCellValue: (r: number, c: number, v: string) => void
): void {
    const { row, col } = selection
    const navTarget = getNavTarget(
        e.key,
        e.shiftKey,
        row,
        col,
        gridDimensions.rows - 1,
        gridDimensions.cols - 1
    )
    if (navTarget) {
        e.preventDefault()
        setSelection(navTarget)
        return
    }

    if (e.key === 'Enter' && !isReadOnly) {
        e.preventDefault()
        startEditing({ row, col })
    } else if ((e.key === 'Delete' || e.key === 'Backspace') && !isReadOnly) {
        e.preventDefault()
        setCellValue(row, col, '')
    } else if (!isReadOnly && e.key.length === 1 && !e.ctrlKey && !e.metaKey) {
        e.preventDefault()
        setCellValue(row, col, '')
        startEditing({ row, col })
    }
}

function getNavTarget(
    key: string,
    shiftKey: boolean,
    row: number,
    col: number,
    maxRow: number,
    maxCol: number
): { row: number; col: number } | null {
    if (key === 'ArrowUp' && row > 0) return { row: row - 1, col }
    if (key === 'ArrowDown' && row < maxRow) return { row: row + 1, col }
    if (key === 'ArrowLeft' && col > 0) return { row, col: col - 1 }
    if (key === 'ArrowRight' && col < maxCol) return { row, col: col + 1 }
    if (key === 'Tab' && shiftKey && col > 0) return { row, col: col - 1 }
    if (key === 'Tab' && !shiftKey && col < maxCol) return { row, col: col + 1 }
    return null
}

export function SpreadsheetGrid() {
    const {
        gridDimensions,
        getColWidth,
        getRowHeight,
        selection,
        editingCell,
        getCellValue,
        setSelection,
        startEditing,
        stopEditing,
        setCellValue,
        isReadOnly,
        undo,
        redo,
    } = useSpreadsheet()

    const theme = useTheme()
    const themeColors = useMemo(
        () => ({
            borderColor: theme.borderColor.val,
            accentBg: theme.accentBackground.val,
            defaultColor: theme.color.val,
            color8: theme.color8.val,
            background: theme.background.val,
        }),
        [
            theme.borderColor.val,
            theme.accentBackground.val,
            theme.color.val,
            theme.color8.val,
            theme.background.val,
        ]
    )

    const {
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
    } = useVirtualGrid({
        numRows: gridDimensions.rows,
        numCols: gridDimensions.cols,
        getColWidth,
        getRowHeight,
    })

    const lastTapRef = useRef<{ row: number; col: number; time: number } | null>(null)

    const hitTestCell = useCallback(
        (locationX: number, locationY: number) => {
            const gridX = locationX - ROW_HEADER_WIDTH
            const gridY = locationY - HEADER_HEIGHT
            if (gridX < 0 || gridY < 0) return null
            const { row, col } = cellFromPoint(gridX, gridY, colOffsets, rowOffsets)
            if (row >= gridDimensions.rows || col >= gridDimensions.cols) return null
            return { row, col }
        },
        [colOffsets, rowOffsets, gridDimensions]
    )

    const handleGridPress = useCallback(
        (e: { nativeEvent: { locationX: number; locationY: number } }) => {
            const hit = hitTestCell(e.nativeEvent.locationX, e.nativeEvent.locationY)
            if (!hit) return

            const now = Date.now()
            const last = lastTapRef.current
            if (last && last.row === hit.row && last.col === hit.col && now - last.time < 400) {
                lastTapRef.current = null
                if (!isReadOnly) startEditing(hit)
                return
            }
            lastTapRef.current = { ...hit, time: now }
            setSelection(hit)
        },
        [hitTestCell, isReadOnly, startEditing, setSelection]
    )

    const handleGridLongPress = useCallback(
        (e: { nativeEvent: { locationX: number; locationY: number } }) => {
            if (isReadOnly) return
            const hit = hitTestCell(e.nativeEvent.locationX, e.nativeEvent.locationY)
            if (hit) startEditing(hit)
        },
        [isReadOnly, hitTestCell, startEditing]
    )

    const handleKeyDown = useCallback(
        (e: React.KeyboardEvent) => {
            if (handleUndoRedoKey(e, undo, redo)) return
            if (editingCell) {
                if (e.key === 'Escape') {
                    e.preventDefault()
                    stopEditing()
                }
                return
            }
            handleGridKey(
                e,
                selection,
                gridDimensions,
                isReadOnly,
                setSelection,
                startEditing,
                setCellValue
            )
        },
        [
            editingCell,
            selection,
            gridDimensions,
            isReadOnly,
            setSelection,
            startEditing,
            stopEditing,
            setCellValue,
            undo,
            redo,
        ]
    )

    const [firstRow, lastRow] = visibleRows
    const [firstCol, lastCol] = visibleCols

    // Visible cells
    const cellNodes = useMemo(() => {
        const result: React.ReactNode[] = []
        for (let r = firstRow; r <= lastRow; r++) {
            const rowTop = rowOffsets[r]
            const rowHeight = getRowHeight(r)
            for (let c = firstCol; c <= lastCol; c++) {
                const colLeft = colOffsets[c]
                const colWidth = getColWidth(c)
                const cell = getCellValue(r, c)
                const isSelected = selection.row === r && selection.col === c
                result.push(
                    <CellRenderer
                        key={`${r}:${c}`}
                        cell={cell}
                        width={colWidth}
                        height={rowHeight}
                        left={ROW_HEADER_WIDTH + colLeft}
                        top={HEADER_HEIGHT + rowTop}
                        isSelected={isSelected}
                        themeColors={themeColors}
                    />
                )
            }
        }
        return result
    }, [
        firstRow,
        lastRow,
        firstCol,
        lastCol,
        rowOffsets,
        colOffsets,
        getRowHeight,
        getColWidth,
        getCellValue,
        selection.row,
        selection.col,
        themeColors,
    ])

    // Visible column headers
    const columnHeaders = useMemo(() => {
        const result: React.ReactNode[] = []
        for (let c = firstCol; c <= lastCol; c++) {
            const colWidth = getColWidth(c)
            const isSelected = selection.col === c
            result.push(
                <View
                    key={c}
                    style={[
                        styles.headerCell,
                        {
                            position: 'absolute',
                            left: ROW_HEADER_WIDTH + colOffsets[c],
                            top: 0,
                            width: colWidth,
                            height: HEADER_HEIGHT,
                            borderRightColor: themeColors.borderColor,
                            borderBottomColor: themeColors.borderColor,
                        },
                        isSelected && { backgroundColor: `${themeColors.accentBg}20` },
                    ]}
                >
                    <Text style={[styles.headerText, { color: themeColors.color8 }]}>
                        {colIndexToLetter(c)}
                    </Text>
                </View>
            )
        }
        return result
    }, [firstCol, lastCol, getColWidth, colOffsets, selection.col, themeColors])

    // Visible row headers
    const rowHeaders = useMemo(() => {
        const result: React.ReactNode[] = []
        for (let r = firstRow; r <= lastRow; r++) {
            const rowHeight = getRowHeight(r)
            const isSelected = selection.row === r
            result.push(
                <View
                    key={r}
                    style={[
                        styles.rowHeaderCell,
                        {
                            position: 'absolute',
                            left: 0,
                            top: HEADER_HEIGHT + rowOffsets[r],
                            width: ROW_HEADER_WIDTH,
                            height: rowHeight,
                            borderRightColor: themeColors.borderColor,
                            borderBottomColor: themeColors.borderColor,
                            backgroundColor: isSelected
                                ? `${themeColors.accentBg}20`
                                : themeColors.background,
                        },
                    ]}
                >
                    <Text style={[styles.headerText, { color: themeColors.color8 }]}>{r + 1}</Text>
                </View>
            )
        }
        return result
    }, [firstRow, lastRow, getRowHeight, rowOffsets, selection.row, themeColors])

    const editingCellPosition = editingCell
        ? {
              left: ROW_HEADER_WIDTH + colOffsets[editingCell.col],
              top: HEADER_HEIGHT + rowOffsets[editingCell.row],
              width: getColWidth(editingCell.col),
              height: getRowHeight(editingCell.row),
          }
        : null

    const contentWidth = totalWidth + ROW_HEADER_WIDTH
    const contentHeight = totalHeight + HEADER_HEIGHT

    const gridContent = (
        <>
            {cellNodes}

            {/* Column headers - frozen row */}
            <View
                style={[
                    styles.colHeaderStrip,
                    {
                        top: scrollY,
                        width: contentWidth,
                        height: HEADER_HEIGHT,
                        backgroundColor: themeColors.background,
                    },
                ]}
            >
                {columnHeaders}
            </View>

            {/* Row headers - frozen column */}
            <View
                style={[
                    styles.rowHeaderStrip,
                    {
                        left: scrollX,
                        height: contentHeight,
                        width: ROW_HEADER_WIDTH,
                        backgroundColor: themeColors.background,
                    },
                ]}
            >
                {rowHeaders}
            </View>

            {/* Corner cell */}
            <View
                style={[
                    styles.cornerCell,
                    {
                        top: scrollY,
                        left: scrollX,
                        width: ROW_HEADER_WIDTH,
                        height: HEADER_HEIGHT,
                        borderRightColor: themeColors.borderColor,
                        borderBottomColor: themeColors.borderColor,
                        backgroundColor: themeColors.background,
                    },
                ]}
            />

            {editingCell && editingCellPosition && (
                <CellEditor
                    row={editingCell.row}
                    col={editingCell.col}
                    width={editingCellPosition.width}
                    height={editingCellPosition.height}
                    left={editingCellPosition.left}
                    top={editingCellPosition.top}
                />
            )}
        </>
    )

    // Web: single div with overflow:auto for native biaxial scroll
    if (isWeb) {
        return (
            <WebScrollContainer
                contentWidth={contentWidth}
                contentHeight={contentHeight}
                handleScrollX={handleScrollX}
                handleScrollY={handleScrollY}
                onLayout={onLayout}
                onPress={handleGridPress}
                onLongPress={handleGridLongPress}
                onKeyDown={handleKeyDown}
            >
                {gridContent}
            </WebScrollContainer>
        )
    }

    // Native: nested ScrollViews (vertical outer, horizontal inner)
    return (
        <View style={styles.container} onLayout={onLayout}>
            <ScrollView
                style={styles.container}
                scrollEventThrottle={16}
                onScroll={e => handleScrollY(e.nativeEvent.contentOffset.y)}
                showsVerticalScrollIndicator
            >
                <ScrollView
                    horizontal
                    scrollEventThrottle={16}
                    onScroll={e => handleScrollX(e.nativeEvent.contentOffset.x)}
                    showsHorizontalScrollIndicator
                >
                    <Pressable
                        style={{ width: contentWidth, height: contentHeight }}
                        onPress={handleGridPress}
                        onLongPress={handleGridLongPress}
                    >
                        {gridContent}
                    </Pressable>
                </ScrollView>
            </ScrollView>
        </View>
    )
}

interface WebScrollContainerProps {
    contentWidth: number
    contentHeight: number
    handleScrollX: (x: number) => void
    handleScrollY: (y: number) => void
    onLayout: (e: { nativeEvent: { layout: { width: number; height: number } } }) => void
    onPress: (e: { nativeEvent: { locationX: number; locationY: number } }) => void
    onLongPress: (e: { nativeEvent: { locationX: number; locationY: number } }) => void
    onKeyDown: (e: React.KeyboardEvent) => void
    children: React.ReactNode
}

function WebScrollContainer({
    contentWidth,
    contentHeight,
    handleScrollX,
    handleScrollY,
    onLayout,
    onPress,
    onLongPress,
    onKeyDown,
    children,
}: WebScrollContainerProps) {
    const handleScroll = useCallback(
        (e: unknown) => {
            const target = (e as { target: HTMLElement }).target
            handleScrollX(target.scrollLeft)
            handleScrollY(target.scrollTop)
        },
        [handleScrollX, handleScrollY]
    )

    return (
        <View style={styles.container} onLayout={onLayout}>
            <View
                // @ts-expect-error overflow:'auto', tabIndex, onKeyDown, onScroll are web-only
                style={[styles.container, { overflow: 'auto' }]}
                onScroll={handleScroll}
                tabIndex={0}
                onKeyDown={onKeyDown}
            >
                <Pressable
                    style={{ width: contentWidth, height: contentHeight }}
                    onPress={onPress}
                    onLongPress={onLongPress}
                >
                    {children}
                </Pressable>
            </View>
        </View>
    )
}

const styles = StyleSheet.create({
    container: {
        flex: 1,
    },
    headerCell: {
        borderRightWidth: 1,
        borderBottomWidth: 1,
        alignItems: 'center',
        justifyContent: 'center',
    },
    headerText: {
        fontSize: 12,
        fontWeight: '600',
    },
    rowHeaderCell: {
        borderRightWidth: 1,
        borderBottomWidth: 1,
        alignItems: 'center',
        justifyContent: 'center',
    },
    colHeaderStrip: {
        position: 'absolute',
        zIndex: 2,
    },
    rowHeaderStrip: {
        position: 'absolute',
        top: 0,
        zIndex: 2,
    },
    cornerCell: {
        position: 'absolute',
        zIndex: 3,
        borderRightWidth: 1,
        borderBottomWidth: 1,
    },
})
