import type PocketBase from 'pocketbase'

function log(...args: unknown[]) {
    process.stdout.write(`[seed:mail] ${args.join(' ')}\n`)
}

interface SeedContext {
    user: { id: string; email: string; name: string }
    org: { id: string }
    userOrg: { id: string }
}

interface SeedMessage {
    sender_name: string
    sender_email: string
    recipients_to: { name: string; email: string }[]
    date: string
    subject: string
    snippet: string
    body_html: string
    in_reply_to?: string
}

function htmlBlob(html: string) {
    return new File([html], 'body.html', { type: 'text/html' })
}

const LABELS = [
    { name: 'Work', color: '#4285f4' },
    { name: 'Personal', color: '#0f9d58' },
    { name: 'Finance', color: '#f4b400' },
    { name: 'Travel', color: '#db4437' },
] as const

const THREADS: {
    subject: string
    snippet: string
    message_count: number
    latest_date: string
    participants: { name: string; email: string }[]
    folder: 'inbox' | 'sent' | 'drafts' | 'trash' | 'spam' | 'archive'
    is_read: boolean
    is_starred: boolean
    labels: string[]
    messages: SeedMessage[]
}[] = [
    {
        subject: 'Q2 Product Roadmap Review',
        snippet:
            "Hi team, I've attached the updated Q2 roadmap with the changes we discussed in yesterday's meeting. Please review and add any comments before Friday.",
        message_count: 3,
        latest_date: '2026-04-04 10:42:00.000Z',
        participants: [
            { name: 'Alice Chen', email: 'alice.chen@tinycld.org' },
            { name: 'Bob Park', email: 'bob.park@tinycld.org' },
        ],
        folder: 'inbox',
        is_read: false,
        is_starred: true,
        labels: ['Work'],
        messages: [
            {
                sender_name: 'Alice Chen',
                sender_email: 'alice.chen@tinycld.org',
                recipients_to: [{ name: 'Team', email: 'support@tinycld.org' }],
                date: '2026-04-04 08:00:00.000Z',
                subject: 'Q2 Product Roadmap Review',
                snippet: 'Hi team, here is the initial Q2 roadmap draft for review.',
                body_html:
                    '<p>Hi team,</p><p>Here is the initial Q2 roadmap draft for review. Please take a look and share your thoughts.</p><p>Best,<br/>Alice</p>',
            },
            {
                sender_name: 'Bob Park',
                sender_email: 'bob.park@tinycld.org',
                recipients_to: [{ name: 'Team', email: 'support@tinycld.org' }],
                in_reply_to: 'roadmap-1@tinycld.org',
                date: '2026-04-04 09:30:00.000Z',
                subject: 'Re: Q2 Product Roadmap Review',
                snippet: 'Looks good overall. I have a few suggestions on the Sprint 14 timeline.',
                body_html:
                    '<p>Looks good overall. I have a few suggestions on the Sprint 14 timeline.</p><p>Can we move the billing migration earlier? The team is ready now.</p><p>Bob</p>',
            },
            {
                sender_name: 'Alice Chen',
                sender_email: 'alice.chen@tinycld.org',
                recipients_to: [{ name: 'Team', email: 'support@tinycld.org' }],
                in_reply_to: 'roadmap-2@tinycld.org',
                date: '2026-04-04 10:42:00.000Z',
                subject: 'Re: Q2 Product Roadmap Review',
                snippet:
                    "I've attached the updated Q2 roadmap with the changes we discussed in yesterday's meeting.",
                body_html:
                    "<p>Hi team,</p><p>I've attached the updated Q2 roadmap with the changes we discussed in yesterday's meeting. Please review and add any comments before Friday.</p><p>Key changes include:</p><ul><li>Moved the billing migration to Sprint 14</li><li>Added the new onboarding flow to Sprint 12</li><li>Deprioritized the analytics dashboard redesign</li></ul><p>Let me know if you have questions.</p><p>Best,<br/>Alice</p>",
            },
        ],
    },
    {
        subject: '[tinycld/app] Fix: resolve subdomain redirect loop (#247)',
        snippet:
            'nathanstitt merged pull request #247 into main. The redirect loop when navigating between org subdomains has been resolved.',
        message_count: 1,
        latest_date: '2026-04-04 09:15:00.000Z',
        participants: [{ name: 'GitHub', email: 'notifications@github.com' }],
        folder: 'inbox',
        is_read: true,
        is_starred: false,
        labels: [],
        messages: [
            {
                sender_name: 'GitHub',
                sender_email: 'notifications@github.com',
                recipients_to: [{ name: 'Alice', email: 'alice@tinycld.org' }],
                date: '2026-04-04 09:15:00.000Z',
                subject: '[tinycld/app] Fix: resolve subdomain redirect loop (#247)',
                snippet: 'nathanstitt merged pull request #247 into main.',
                body_html:
                    '<p><strong>nathanstitt</strong> merged pull request <a href="#">#247</a> into <code>main</code>.</p><p>The redirect loop when navigating between org subdomains has been resolved by checking the current hostname before triggering a redirect.</p><p>Files changed: 3<br/>Additions: 42<br/>Deletions: 18</p>',
            },
        ],
    },
    {
        subject: 'Lunch tomorrow?',
        snippet:
            "Hey! Are you free for lunch tomorrow around noon? There's a new Thai place on 5th that just opened.",
        message_count: 1,
        latest_date: '2026-04-03 12:30:00.000Z',
        participants: [{ name: 'Marcus Johnson', email: 'marcus.j@tinycld.org' }],
        folder: 'inbox',
        is_read: true,
        is_starred: false,
        labels: ['Personal'],
        messages: [
            {
                sender_name: 'Marcus Johnson',
                sender_email: 'marcus.j@tinycld.org',
                recipients_to: [{ name: 'Alice', email: 'alice@tinycld.org' }],
                date: '2026-04-03 12:30:00.000Z',
                subject: 'Lunch tomorrow?',
                snippet: 'Hey! Are you free for lunch tomorrow around noon?',
                body_html:
                    "<p>Hey!</p><p>Are you free for lunch tomorrow around noon? There's a new Thai place on 5th that just opened. Heard great things about their pad see ew.</p><p>Let me know!</p><p>Marcus</p>",
            },
        ],
    },
    {
        subject: 'Your receipt from TinyCld',
        snippet:
            'Payment of $49.00 for TinyCld Pro Plan (Monthly) was successfully processed on April 1, 2026.',
        message_count: 1,
        latest_date: '2026-04-01 14:00:00.000Z',
        participants: [{ name: 'Stripe', email: 'receipts@stripe.com' }],
        folder: 'inbox',
        is_read: true,
        is_starred: false,
        labels: ['Finance'],
        messages: [
            {
                sender_name: 'Stripe',
                sender_email: 'receipts@stripe.com',
                recipients_to: [{ name: 'Alice', email: 'alice@tinycld.org' }],
                date: '2026-04-01 14:00:00.000Z',
                subject: 'Your receipt from TinyCld',
                snippet:
                    'Payment of $49.00 for TinyCld Pro Plan (Monthly) was successfully processed.',
                body_html:
                    '<p>Payment of <strong>$49.00</strong> for TinyCld Pro Plan (Monthly) was successfully processed on April 1, 2026.</p><p>Invoice #INV-2026-0401</p><p>If you have any questions about this charge, please contact support.</p>',
            },
        ],
    },
    {
        subject: 'Re: Conference travel arrangements',
        snippet:
            "I've booked the flights for the team. Departing SFO on the 15th at 8:30 AM, returning on the 18th.",
        message_count: 5,
        latest_date: '2026-03-30 16:45:00.000Z',
        participants: [
            { name: 'Sarah Kim', email: 'sarah.kim@tinycld.org' },
            { name: 'Alice', email: 'alice@tinycld.org' },
            { name: 'David Lee', email: 'david.lee@tinycld.org' },
        ],
        folder: 'inbox',
        is_read: false,
        is_starred: true,
        labels: ['Work', 'Travel'],
        messages: [
            {
                sender_name: 'David Lee',
                sender_email: 'david.lee@tinycld.org',
                recipients_to: [
                    { name: 'Sarah Kim', email: 'sarah.kim@tinycld.org' },
                    { name: 'Alice', email: 'alice@tinycld.org' },
                ],
                date: '2026-03-28 09:00:00.000Z',
                subject: 'Conference travel arrangements',
                snippet: 'Hi all, we need to book travel for the NYC conference.',
                body_html:
                    '<p>Hi all,</p><p>We need to book travel for the NYC conference April 15-18. Can someone take the lead on flights and hotel?</p><p>David</p>',
            },
            {
                sender_name: 'Alice',
                sender_email: 'alice@tinycld.org',
                recipients_to: [
                    { name: 'Sarah Kim', email: 'sarah.kim@tinycld.org' },
                    { name: 'David Lee', email: 'david.lee@tinycld.org' },
                ],
                in_reply_to: 'travel-1@tinycld.org',
                date: '2026-03-28 10:15:00.000Z',
                subject: 'Re: Conference travel arrangements',
                snippet: 'I can help coordinate. Sarah, can you handle flights?',
                body_html:
                    "<p>I can help coordinate. Sarah, can you handle flights? I'll look into hotels.</p><p>Alice</p>",
            },
            {
                sender_name: 'Sarah Kim',
                sender_email: 'sarah.kim@tinycld.org',
                recipients_to: [
                    { name: 'Alice', email: 'alice@tinycld.org' },
                    { name: 'David Lee', email: 'david.lee@tinycld.org' },
                ],
                in_reply_to: 'travel-2@tinycld.org',
                date: '2026-03-29 11:00:00.000Z',
                subject: 'Re: Conference travel arrangements',
                snippet: 'On it! Looking at flights now.',
                body_html:
                    '<p>On it! Looking at flights now. Will share options by end of day.</p><p>Sarah</p>',
            },
            {
                sender_name: 'Alice',
                sender_email: 'alice@tinycld.org',
                recipients_to: [
                    { name: 'Sarah Kim', email: 'sarah.kim@tinycld.org' },
                    { name: 'David Lee', email: 'david.lee@tinycld.org' },
                ],
                in_reply_to: 'travel-3@tinycld.org',
                date: '2026-03-30 08:30:00.000Z',
                subject: 'Re: Conference travel arrangements',
                snippet:
                    "I've booked the Marriott Marquis. Confirmation numbers coming separately.",
                body_html:
                    "<p>I've booked the Marriott Marquis in Times Square for all three of us. Confirmation numbers coming separately.</p><p>Alice</p>",
            },
            {
                sender_name: 'Sarah Kim',
                sender_email: 'sarah.kim@tinycld.org',
                recipients_to: [
                    { name: 'Alice', email: 'alice@tinycld.org' },
                    { name: 'David Lee', email: 'david.lee@tinycld.org' },
                ],
                in_reply_to: 'travel-4@tinycld.org',
                date: '2026-03-30 16:45:00.000Z',
                subject: 'Re: Conference travel arrangements',
                snippet:
                    "I've booked the flights for the team. Departing SFO on the 15th at 8:30 AM.",
                body_html:
                    "<p>I've booked the flights for the team. Here are the details:</p><p><strong>Departure:</strong> SFO → JFK, April 15, 8:30 AM<br/><strong>Return:</strong> JFK → SFO, April 18, 6:15 PM</p><p>Hotel reservations are at the Marriott Marquis in Times Square. I'll send the confirmation numbers separately.</p><p>Sarah</p>",
            },
        ],
    },
    {
        subject: 'Draft: Monthly newsletter',
        snippet:
            "Here's the draft for this month's newsletter. Still need to add the section about new features.",
        message_count: 1,
        latest_date: '2026-03-28 15:00:00.000Z',
        participants: [{ name: 'Alice', email: 'alice@tinycld.org' }],
        folder: 'drafts',
        is_read: true,
        is_starred: false,
        labels: [],
        messages: [
            {
                sender_name: 'Alice',
                sender_email: 'alice@tinycld.org',
                recipients_to: [],
                date: '2026-03-28 15:00:00.000Z',
                subject: 'Draft: Monthly newsletter',
                snippet: "Here's the draft for this month's newsletter.",
                body_html:
                    "<p>Here's the draft for this month's newsletter. Still need to add the section about new features and the customer spotlight.</p><p>TODO:</p><ul><li>Add new features section</li><li>Customer spotlight: Acme Corp</li><li>Proofread and finalize</li></ul>",
            },
        ],
    },
    {
        subject: 'Team standup notes — March 27',
        snippet:
            "Quick summary from today's standup: API migration on track, mobile app release pushed to next week.",
        message_count: 1,
        latest_date: '2026-03-27 17:00:00.000Z',
        participants: [{ name: 'Alice', email: 'alice@tinycld.org' }],
        folder: 'sent',
        is_read: true,
        is_starred: false,
        labels: ['Work'],
        messages: [
            {
                sender_name: 'Alice',
                sender_email: 'alice@tinycld.org',
                recipients_to: [{ name: 'Team', email: 'support@tinycld.org' }],
                date: '2026-03-27 17:00:00.000Z',
                subject: 'Team standup notes — March 27',
                snippet: "Quick summary from today's standup.",
                body_html:
                    "<p>Quick summary from today's standup:</p><ul><li>API migration on track for Friday deploy</li><li>Mobile app release pushed to next week due to App Store review delays</li><li>New hire starting Monday — onboarding buddy: Marcus</li></ul>",
            },
        ],
    },
]

function deriveAddress(email: string) {
    return (
        email
            .split('@')[0]
            .toLowerCase()
            .replace(/[^a-z0-9._-]/g, '') || 'user'
    )
}

async function findUniqueAddress(pb: PocketBase, base: string, domainId: string) {
    let address = base
    let suffix = 2
    while (true) {
        try {
            await pb
                .collection('mail_mailboxes')
                .getFirstListItem(`address = "${address}" && domain = "${domainId}"`)
            address = `${base}${suffix++}`
        } catch {
            return address
        }
    }
}

async function findOrCreatePersonalMailbox(
    pb: PocketBase,
    email: string,
    name: string,
    domainId: string,
    userOrgId: string
) {
    const base = deriveAddress(email)

    // Check if mailbox already exists
    try {
        const existing = await pb
            .collection('mail_mailboxes')
            .getFirstListItem(`address = "${base}" && domain = "${domainId}"`)
        return existing
    } catch {
        // Not found, create it
    }

    const address = await findUniqueAddress(pb, base, domainId)
    const mailbox = await pb.collection('mail_mailboxes').create({
        address,
        domain: domainId,
        display_name: name || address,
        type: 'personal',
    })

    await pb.collection('mail_mailbox_members').create({
        mailbox: mailbox.id,
        user_org: userOrgId,
        role: 'owner',
    })

    return mailbox
}

export default async function seed(pb: PocketBase, { user, org, userOrg }: SeedContext) {
    let domain: { id: string }
    try {
        domain = await pb
            .collection('mail_domains')
            .getFirstListItem(`org = "${org.id}" && domain = "tinycld.org"`)
        log('Found existing mail domain: tinycld.org')
    } catch {
        log('Creating mail domain: tinycld.org')
        domain = await pb.collection('mail_domains').create({
            org: org.id,
            domain: 'tinycld.org',
            verified: true,
        })
    }

    log('Setting up personal mailbox for', user.email)
    const personalMailbox = await findOrCreatePersonalMailbox(
        pb,
        user.email,
        user.name,
        domain.id,
        userOrg.id
    )

    let sharedMailbox: { id: string }
    try {
        sharedMailbox = await pb
            .collection('mail_mailboxes')
            .getFirstListItem(`address = "support" && domain = "${domain.id}"`)
        log('Found existing shared mailbox: support')
    } catch {
        log('Creating shared mailbox: support')
        sharedMailbox = await pb.collection('mail_mailboxes').create({
            address: 'support',
            domain: domain.id,
            display_name: 'Support',
            type: 'shared',
        })

        await pb.collection('mail_mailbox_members').create({
            mailbox: sharedMailbox.id,
            user_org: userOrg.id,
            role: 'owner',
        })
    }

    // Create personal mailboxes for any other org members
    const otherMembers = await pb.collection('user_org').getFullList({
        filter: `org = "${org.id}" && id != "${userOrg.id}"`,
        expand: 'user',
    })
    if (otherMembers.length > 0) {
        log(`Setting up mailboxes for ${otherMembers.length} other org member(s)`)
    }
    for (const member of otherMembers) {
        const memberUser = member.expand?.user as { email: string; name: string } | undefined
        if (!memberUser) continue
        await findOrCreatePersonalMailbox(
            pb,
            memberUser.email,
            memberUser.name,
            domain.id,
            member.id
        )
    }

    // Create labels (find or create)
    log(`Setting up ${LABELS.length} labels...`)
    const labelMap: Record<string, string> = {}
    for (const label of LABELS) {
        let record: { id: string }
        try {
            record = await pb
                .collection('mail_labels')
                .getFirstListItem(`org = "${org.id}" && name = "${label.name}"`)
        } catch {
            record = await pb.collection('mail_labels').create({
                org: org.id,
                name: label.name,
                color: label.color,
            })
        }
        labelMap[label.name] = record.id
    }

    // Check if threads already exist for this mailbox
    const existingThreads = await pb.collection('mail_threads').getList(1, 1, {
        filter: `mailbox = "${personalMailbox.id}"`,
    })
    if (existingThreads.totalItems > 0) {
        log(`Skipping threads (${existingThreads.totalItems} already exist)`)
        return
    }

    log(`Creating ${THREADS.length} threads with messages...`)
    for (const thread of THREADS) {
        const threadRecord = await pb.collection('mail_threads').create({
            mailbox: personalMailbox.id,
            subject: thread.subject,
            snippet: thread.snippet,
            message_count: thread.message_count,
            latest_date: thread.latest_date,
            participants: thread.participants,
        })

        for (let i = 0; i < thread.messages.length; i++) {
            const msg = thread.messages[i]
            const formData = new FormData()
            formData.append('thread', threadRecord.id)
            formData.append('sender_name', msg.sender_name)
            formData.append('sender_email', msg.sender_email)
            formData.append('recipients_to', JSON.stringify(msg.recipients_to))
            formData.append('recipients_cc', JSON.stringify([]))
            formData.append('date', msg.date)
            formData.append('subject', msg.subject)
            formData.append('snippet', msg.snippet)
            formData.append('has_attachments', 'false')
            formData.append('body_html', htmlBlob(msg.body_html))

            if (msg.in_reply_to) {
                formData.append('in_reply_to', msg.in_reply_to)
            }

            const msgId = `<${thread.subject
                .toLowerCase()
                .replace(/[^a-z0-9]/g, '-')
                .slice(0, 30)}-${i + 1}@tinycld.org>`
            formData.append('message_id', msgId)

            await pb.collection('mail_messages').create(formData)
        }

        const labelIds = thread.labels.map(name => labelMap[name]).filter(Boolean)

        await pb.collection('mail_thread_state').create({
            thread: threadRecord.id,
            user_org: userOrg.id,
            folder: thread.folder,
            is_read: thread.is_read,
            is_starred: thread.is_starred,
            labels: labelIds,
        })
    }

    log(`Created ${THREADS.length} threads`)
}
