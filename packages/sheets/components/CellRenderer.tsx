import { memo } from 'react'
import { StyleSheet, Text, View } from 'react-native'
import type { CellData } from '../lib/cell-utils'

export interface ThemeColors {
    borderColor: string
    accentBg: string
    defaultColor: string
    color8: string
    background: string
}

interface CellRendererProps {
    cell: CellData | undefined
    width: number
    height: number
    left: number
    top: number
    isSelected: boolean
    themeColors: ThemeColors
}

export const CellRenderer = memo(function CellRenderer({
    cell,
    width,
    height,
    left,
    top,
    isSelected,
    themeColors,
}: CellRendererProps) {
    const displayValue = cell?.computed ?? cell?.value ?? ''
    const align = cell?.align ?? (cell?.type === 'number' ? 'right' : 'left')

    return (
        <View
            style={[
                styles.cell,
                {
                    position: 'absolute',
                    left,
                    top,
                    width,
                    height,
                    borderRightColor: themeColors.borderColor,
                    borderBottomColor: themeColors.borderColor,
                    backgroundColor: cell?.bgColor ?? 'transparent',
                },
                isSelected && {
                    borderColor: themeColors.accentBg,
                    borderWidth: 2,
                },
            ]}
        >
            <Text
                style={[
                    styles.cellText,
                    {
                        color: cell?.textColor ?? themeColors.defaultColor,
                        textAlign: align,
                        fontWeight: cell?.bold ? '700' : '400',
                        fontStyle: cell?.italic ? 'italic' : 'normal',
                    },
                ]}
                numberOfLines={1}
            >
                {displayValue}
            </Text>
        </View>
    )
})

const styles = StyleSheet.create({
    cell: {
        borderRightWidth: 1,
        borderBottomWidth: 1,
        justifyContent: 'center',
        paddingHorizontal: 4,
    },
    cellText: {
        fontSize: 13,
    },
})
