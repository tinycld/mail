// Deleting a domain cascades to its mailboxes, so the confirmation must say so.
// It names no count: an admin can only list mailboxes they belong to.
export function domainRemovalWarning(domain: string) {
    return `Removing ${domain} also deletes every mailbox on it and all their mail. This cannot be undone.`
}
