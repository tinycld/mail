import {
    AlignCenter,
    AlignLeft,
    AlignRight,
    Bold,
    Download,
    Italic,
    Redo2,
    Undo2,
    Upload,
} from 'lucide-react-native'
import { Pressable, StyleSheet, View } from 'react-native'
import { useTheme } from 'tamagui'
import { useSpreadsheet } from '../hooks/useSpreadsheet'

export function SpreadsheetToolbar() {
    const theme = useTheme()
    const { selection, getCellValue, setCellFormat, isReadOnly, undo, redo, canUndo, canRedo } =
        useSpreadsheet()

    const cell = getCellValue(selection.row, selection.col)

    const toggleBold = () => {
        setCellFormat(selection.row, selection.col, { bold: !cell?.bold })
    }

    const toggleItalic = () => {
        setCellFormat(selection.row, selection.col, { italic: !cell?.italic })
    }

    const setAlign = (align: 'left' | 'center' | 'right') => {
        setCellFormat(selection.row, selection.col, { align })
    }

    const iconColor = theme.color8.val

    return (
        <View style={[styles.container, { borderBottomColor: theme.borderColor.val }]}>
            {!isReadOnly && (
                <View style={styles.group}>
                    <ToolbarButton
                        icon={Undo2}
                        onPress={undo}
                        color={iconColor}
                        disabled={!canUndo}
                    />
                    <ToolbarButton
                        icon={Redo2}
                        onPress={redo}
                        color={iconColor}
                        disabled={!canRedo}
                    />
                    <View style={[styles.separator, { backgroundColor: theme.borderColor.val }]} />
                    <ToolbarButton
                        icon={Bold}
                        onPress={toggleBold}
                        isActive={cell?.bold}
                        color={iconColor}
                        activeColor={theme.accentBackground.val}
                    />
                    <ToolbarButton
                        icon={Italic}
                        onPress={toggleItalic}
                        isActive={cell?.italic}
                        color={iconColor}
                        activeColor={theme.accentBackground.val}
                    />
                    <View style={[styles.separator, { backgroundColor: theme.borderColor.val }]} />
                    <ToolbarButton
                        icon={AlignLeft}
                        onPress={() => setAlign('left')}
                        isActive={cell?.align === 'left'}
                        color={iconColor}
                        activeColor={theme.accentBackground.val}
                    />
                    <ToolbarButton
                        icon={AlignCenter}
                        onPress={() => setAlign('center')}
                        isActive={cell?.align === 'center'}
                        color={iconColor}
                        activeColor={theme.accentBackground.val}
                    />
                    <ToolbarButton
                        icon={AlignRight}
                        onPress={() => setAlign('right')}
                        isActive={cell?.align === 'right'}
                        color={iconColor}
                        activeColor={theme.accentBackground.val}
                    />
                </View>
            )}
            <View style={styles.spacer} />
            <View style={styles.group}>
                <ToolbarButton icon={Upload} onPress={() => {}} color={iconColor} />
                <ToolbarButton icon={Download} onPress={() => {}} color={iconColor} />
            </View>
        </View>
    )
}

interface ToolbarButtonProps {
    icon: React.ComponentType<{ size: number; color: string }>
    onPress: () => void
    isActive?: boolean
    color: string
    activeColor?: string
    disabled?: boolean
}

function ToolbarButton({
    icon: Icon,
    onPress,
    isActive,
    color,
    activeColor,
    disabled,
}: ToolbarButtonProps) {
    return (
        <Pressable
            onPress={onPress}
            disabled={disabled}
            style={[
                styles.button,
                isActive && activeColor ? { backgroundColor: `${activeColor}20` } : undefined,
                disabled && styles.disabled,
            ]}
        >
            <Icon size={16} color={isActive && activeColor ? activeColor : color} />
        </Pressable>
    )
}

const styles = StyleSheet.create({
    container: {
        flexDirection: 'row',
        alignItems: 'center',
        paddingHorizontal: 8,
        height: 36,
        borderBottomWidth: 1,
    },
    group: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 2,
    },
    separator: {
        width: 1,
        height: 20,
        marginHorizontal: 6,
    },
    spacer: {
        flex: 1,
    },
    button: {
        padding: 6,
        borderRadius: 4,
    },
    disabled: {
        opacity: 0.3,
    },
})
