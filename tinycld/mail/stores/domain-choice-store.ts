import { asyncStorage, create, persist } from '@tinycld/core/lib/store'
import type { DomainChoice } from '../setup/domain-choice'

interface DomainChoiceState {
    choice: DomainChoice | null
    setChoice: (choice: DomainChoice) => void
}

// The Email domain step unmounts when the wizard moves on, so the tab the
// person picked must outlive it; otherwise going back reopens the default tab.
export const useDomainChoiceStore = create<DomainChoiceState>()(
    persist(
        set => ({
            choice: null,
            setChoice: choice => set({ choice }),
        }),
        {
            name: 'tinycld_mail_setup_domain_choice',
            storage: asyncStorage,
            partialize: s => ({ choice: s.choice }),
        }
    )
)
