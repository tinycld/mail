const STAGE_TEXT: Record<string, string> = {
    metadata: 'Metadata',
    full: 'Full text',
}

const ERROR_TEXT: Record<string, string> = {
    budget_exceeded: 'Budget reached',
}

const ACTION_TEXT: Record<string, string> = {
    label: 'Labelled',
    label_archive: 'Labelled and archived',
}

export interface ClassificationFacts {
    confidence: number | null | undefined
    stage: string
    action: string
    preview: boolean
    error: string
}

export function confidenceText(confidence: number | null | undefined) {
    return `${Math.round((confidence ?? 0) * 100)}%`
}

export function stageText(stage: string) {
    return STAGE_TEXT[stage] ?? ''
}

// A preview run never changes the message, whatever the stored action says,
// and an error row has no action at all.
export function actionText({ error, preview, action }: ClassificationFacts) {
    if (error) return ERROR_TEXT[error] ?? 'Not classified'
    if (preview) return 'Not changed (preview)'
    return ACTION_TEXT[action] ?? 'Not changed'
}
