import { pb } from '@tinycld/core/lib/pocketbase'

// fileToken authenticates the rewritten URLs. The message's attachments sit
// behind the record's viewRule, and these <img> tags load from inside a
// sandboxed iframe that carries no ambient credentials — so each URL needs an
// explicit ?token=, the same one the body fetch uses.
export function rewriteCidReferences(
    html: string,
    collectionId: string,
    recordId: string,
    cidMap: Record<string, string> | null | undefined,
    fileToken?: string
): string {
    if (!cidMap || Object.keys(cidMap).length === 0) return html
    return html.replace(
        /(<img[^>]*src=["'])cid:([^"']+)(["'][^>]*>)/gi,
        (match, prefix, cid, suffix) => {
            const normalized = cid.trim().toLowerCase().replace(/^<|>$/g, '')
            const filename = cidMap[normalized]
            if (!filename) return match
            const url = pb.files.getURL(
                { collectionId, id: recordId },
                filename,
                fileToken ? { token: fileToken } : undefined
            )
            return `${prefix}${url}${suffix}`
        }
    )
}
