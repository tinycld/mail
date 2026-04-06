import type { ComponentType, ReactNode } from 'react'
import { addonProviders } from '~/lib/generated/addon-providers'

const stableProviderChain: ComponentType<{ children: ReactNode }>[] = Object.values(
    addonProviders
).filter((p): p is ComponentType<{ children: ReactNode }> => p != null)

export function AddonProviderWrapper({ children }: { children: ReactNode }) {
    return stableProviderChain.reduceRight<ReactNode>(
        (acc, Provider) => <Provider key={Provider.displayName || Provider.name}>{acc}</Provider>,
        children
    )
}
