import { create } from '@tinycld/core/lib/store'
import type { AdvancedSearchFilters } from '../hooks/useSearchState'

interface AdvancedSearchState {
    isOpen: boolean
    /**
     * Values to fill the panel with in place of the applied filters, set when
     * another screen opens it ("Filter messages like these"). The panel only
     * fills its fields; nothing is searched until the user presses Search.
     */
    draft: AdvancedSearchFilters | null
    setOpen: (isOpen: boolean) => void
    openWith: (draft: AdvancedSearchFilters) => void
}

export const useAdvancedSearchStore = create<AdvancedSearchState>(set => ({
    isOpen: false,
    draft: null,
    setOpen: isOpen => set({ isOpen, draft: null }),
    openWith: draft => set({ isOpen: true, draft }),
}))
