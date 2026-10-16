import type { ReactNode } from 'react'

// react-native-drax pulls in gesture-handler and worklets native setup, which
// cannot load under Node. Inert pass-through containers are enough for tests
// that import a screen containing a SortableList but do not drag.
const passThrough = ({ children }: { children?: ReactNode }) => <>{children}</>

export const DraxHandle = passThrough
export const SortableContainer = passThrough
export const SortableItem = passThrough
export function useSortableList<T>({ data }: { data: T[] }) {
    return { data, stableKeyExtractor: (_item: T, index: number) => String(index) }
}
