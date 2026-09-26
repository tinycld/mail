// Deleting a domain cascades to its mailboxes, so the confirmation must say so.
export function domainRemovalWarning(domain: string, mailboxCount: number) {
    if (mailboxCount === 0) return `Removing ${domain} stops mail for it. This cannot be undone.`
    const mailboxes = mailboxCount === 1 ? '1 mailbox' : `${mailboxCount} mailboxes`
    const owner = mailboxCount === 1 ? 'its' : 'their'
    return `Removing ${domain} also deletes its ${mailboxes} and all ${owner} mail. This cannot be undone.`
}
