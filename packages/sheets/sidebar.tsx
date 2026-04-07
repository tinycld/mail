import { Clock, Plus, Table, Users } from 'lucide-react-native'
import {
    SidebarActionButton,
    SidebarDivider,
    SidebarItem,
    SidebarNav,
} from '~/components/sidebar-primitives'
import { useSheets } from './hooks/useSheets'

interface SheetsSidebarProps {
    isCollapsed: boolean
}

export default function SheetsSidebar(_props: SheetsSidebarProps) {
    const { activeSection, navigateToSection, createWorkbook } = useSheets()

    const handleNew = () => {
        createWorkbook.mutate({ name: 'Untitled spreadsheet' })
    }

    return (
        <SidebarNav>
            <SidebarActionButton label="New" icon={Plus} onPress={handleNew} />

            <SidebarItem
                label="My Sheets"
                icon={Table}
                isActive={activeSection === 'my-sheets'}
                onPress={() => navigateToSection('my-sheets')}
            />

            <SidebarDivider />

            <SidebarItem
                label="Shared with me"
                icon={Users}
                isActive={activeSection === 'shared-with-me'}
                onPress={() => navigateToSection('shared-with-me')}
            />
            <SidebarItem
                label="Recent"
                icon={Clock}
                isActive={activeSection === 'recent'}
                onPress={() => navigateToSection('recent')}
            />
        </SidebarNav>
    )
}
