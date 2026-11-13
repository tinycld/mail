import { DocumentTitle } from '@tinycld/core/components/DocumentTitle'
import { HelpIcon } from '@tinycld/core/components/help/HelpIcon'
import { RulesPanel } from '@tinycld/core/components/rules/RulesPanel'
import type { CatalogResponse } from '@tinycld/core/lib/automation/api'
import { usePackages } from '@tinycld/core/lib/packages/use-packages'
import { useCallback } from 'react'
import { Text, View } from 'react-native'

// "Is in my contacts" has no answer without the contacts package, and the
// server leaves the fact absent then, so offering it would build a rule that
// can never match.
const CONTACTS_FIELD = 'fields.sender_in_contacts'

function withoutContactsFact(catalog: CatalogResponse): CatalogResponse {
    return {
        ...catalog,
        triggers: catalog.triggers.map(trigger => ({
            ...trigger,
            fields: trigger.fields?.filter(field => field.key !== CONTACTS_FIELD),
        })),
    }
}

function useCatalogFilter() {
    const hasContacts = usePackages().some(p => p.slug === 'contacts')
    return useCallback(
        (catalog: CatalogResponse) => (hasContacts ? catalog : withoutContactsFact(catalog)),
        [hasContacts]
    )
}

export default function MailRulesScreen() {
    const filterCatalog = useCatalogFilter()
    return (
        <View className="flex-1 p-5 bg-background">
            <DocumentTitle pkg="Mail" title="Rules" />
            <View className="flex-row items-center gap-2 mb-4">
                <Text className="text-2xl font-bold text-foreground">Mail rules</Text>
                <HelpIcon topic="mail:rules" size={18} />
            </View>
            <RulesPanel scope="personal" pkgFilter="mail" canEdit filterCatalog={filterCatalog} />
        </View>
    )
}
