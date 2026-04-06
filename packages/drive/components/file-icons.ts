import {
    File,
    FileSpreadsheet,
    FileText,
    Folder,
    Image,
    type LucideIcon,
    Palette,
    Presentation,
} from 'lucide-react-native'
import type { DriveItemType } from '../types'

export interface FileIconConfig {
    icon: LucideIcon
    color: string
}

interface FileIconEntry {
    icon: LucideIcon
    brandColor?: string
}

const fileIconMap: Record<DriveItemType, FileIconEntry> = {
    folder: { icon: Folder },
    document: { icon: FileText, brandColor: '#4285F4' },
    spreadsheet: { icon: FileSpreadsheet, brandColor: '#0F9D58' },
    pdf: { icon: FileText, brandColor: '#EA4335' },
    image: { icon: Image, brandColor: '#F5A623' },
    presentation: { icon: Presentation, brandColor: '#FBBC04' },
    drawing: { icon: Palette, brandColor: '#EA4335' },
}

export function getFileIcon(type: DriveItemType, neutralColor: string): FileIconConfig {
    const entry = fileIconMap[type] ?? { icon: File }
    return { icon: entry.icon, color: entry.brandColor ?? neutralColor }
}
