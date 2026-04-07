import { ChevronLeft, ChevronRight, Download, X } from 'lucide-react-native'
import { useCallback, useMemo } from 'react'
import { Platform, Pressable, StyleSheet, View } from 'react-native'
import { Dialog, SizableText, useTheme } from 'tamagui'
import { useDrive } from '../hooks/useDrive'
import { getPreviewEntry } from '../lib/preview-registry'
import type { DriveItemView } from '../types'
import { GenericPreview } from './previews/GenericPreview'

interface PreviewModalProps {
    isVisible: boolean
    item: DriveItemView | null
    onClose: () => void
}

export function PreviewModal({ isVisible, item, onClose }: PreviewModalProps) {
    if (!isVisible || !item) return null
    return <PreviewModalContent item={item} onClose={onClose} />
}

function PreviewModalContent({ item, onClose }: { item: DriveItemView; onClose: () => void }) {
    const theme = useTheme()
    const { currentItems, openPreview, downloadItem } = useDrive()

    const files = useMemo(() => currentItems.filter(i => !i.isFolder), [currentItems])
    const currentIndex = files.findIndex(f => f.id === item.id)

    const handlePrevious = useCallback(() => {
        if (currentIndex > 0) openPreview(files[currentIndex - 1])
    }, [currentIndex, files, openPreview])

    const handleNext = useCallback(() => {
        if (currentIndex < files.length - 1) openPreview(files[currentIndex + 1])
    }, [currentIndex, files, openPreview])

    const hasPrevious = currentIndex > 0
    const hasNext = currentIndex < files.length - 1

    const entry = getPreviewEntry(item.mimeType)
    const PreviewComponent = entry?.preview ?? GenericPreview

    const handleDownload = useCallback(() => {
        downloadItem(item.id)
    }, [downloadItem, item.id])

    return (
        <Dialog
            modal
            open
            onOpenChange={open => {
                if (!open) onClose()
            }}
        >
            <Dialog.Portal>
                <Dialog.Overlay
                    key="overlay"
                    opacity={0.6}
                    backgroundColor="$shadow6"
                    enterStyle={{ opacity: 0 }}
                    exitStyle={{ opacity: 0 }}
                />
                <Dialog.Content
                    key="content"
                    backgroundColor="$background"
                    borderRadius={12}
                    padding={0}
                    {...(Platform.OS === 'web'
                        ? { width: '95vw' as never, height: '90vh' as never, maxWidth: 1400 }
                        : { flex: 1 })}
                >
                    <View style={[styles.header, { borderBottomColor: theme.borderColor.val }]}>
                        <SizableText
                            size="$4"
                            fontWeight="600"
                            color="$color"
                            numberOfLines={1}
                            flex={1}
                        >
                            {item.name}
                        </SizableText>
                        <View style={styles.headerActions}>
                            {hasPrevious && (
                                <Pressable
                                    onPress={handlePrevious}
                                    style={styles.headerButton}
                                    hitSlop={8}
                                >
                                    <ChevronLeft size={20} color={theme.color8.val} />
                                </Pressable>
                            )}
                            {hasNext && (
                                <Pressable
                                    onPress={handleNext}
                                    style={styles.headerButton}
                                    hitSlop={8}
                                >
                                    <ChevronRight size={20} color={theme.color8.val} />
                                </Pressable>
                            )}
                            <Pressable
                                onPress={handleDownload}
                                style={styles.headerButton}
                                hitSlop={8}
                            >
                                <Download size={18} color={theme.color8.val} />
                            </Pressable>
                            <Pressable onPress={onClose} style={styles.headerButton} hitSlop={8}>
                                <X size={20} color={theme.color8.val} />
                            </Pressable>
                        </View>
                    </View>
                    <View style={styles.body}>
                        <PreviewComponent
                            item={item}
                            onClose={onClose}
                            onNext={hasNext ? handleNext : undefined}
                            onPrevious={hasPrevious ? handlePrevious : undefined}
                        />
                    </View>
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog>
    )
}

const styles = StyleSheet.create({
    header: {
        flexDirection: 'row',
        alignItems: 'center',
        paddingHorizontal: 16,
        paddingVertical: 12,
        borderBottomWidth: 1,
        gap: 12,
    },
    headerActions: {
        flexDirection: 'row',
        alignItems: 'center',
        gap: 4,
    },
    headerButton: {
        padding: 6,
        borderRadius: 6,
    },
    body: {
        flex: 1,
        overflow: 'hidden',
    },
})
