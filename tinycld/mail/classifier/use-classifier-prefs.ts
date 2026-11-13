import { useUserPreference } from '@tinycld/core/lib/use-user-preference'

export type ClassifierMode = 'off' | 'preview' | 'labels' | 'labels_archive'
export type TaxonomyChoice = 'org' | 'personal'

export function useClassifierPrefs() {
    const [mode, setMode] = useUserPreference<ClassifierMode>('mail', 'classifier.mode', 'off')
    const [taxonomy, setTaxonomy] = useUserPreference<TaxonomyChoice>(
        'mail',
        'classifier.taxonomy',
        'org'
    )
    return { mode, setMode, taxonomy, setTaxonomy }
}
