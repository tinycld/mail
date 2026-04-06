import { type ReactNode, useContext } from 'react'
import { DriveContext, useDriveState } from './hooks/useDrive'

export default function DriveProvider({ children }: { children: ReactNode }) {
    const existing = useContext(DriveContext)
    if (existing) return <>{children}</>

    return <DriveProviderInner>{children}</DriveProviderInner>
}

function DriveProviderInner({ children }: { children: ReactNode }) {
    const state = useDriveState()
    return <DriveContext.Provider value={state}>{children}</DriveContext.Provider>
}
