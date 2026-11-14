// Adapted from jevMail by Ilia AGI (MIT): https://github.com/ilyamk/jev-gmail-ai-spam-filter-and-labeling
import type { TaxonomyPreset } from '@tinycld/core/lib/classifier/taxonomy-schema'

export const MAIL_TAXONOMY_PRESETS: TaxonomyPreset[] = [
    {
        id: 'universal',
        label: 'Universal inbox',
        rows: [
            {
                name: 'action-required',
                assignWhen:
                    'A legitimate message that requires a reply, decision, approval, task, or time-sensitive intervention. Includes unresolved access, security, payment, delivery, legal, or account problems. Excludes optional reading and routine confirmations.',
                autoAction: false,
            },
            {
                name: 'important-update',
                assignWhen:
                    'A meaningful update from a trusted person, project, customer, employer, school, service, or account that should be retained, but does not currently require action.',
                autoAction: false,
            },
            {
                name: 'personal',
                assignWhen:
                    'Person-to-person conversation with family, friends, or a known community contact. Excludes business automation, transactions, and unsolicited commercial outreach.',
                autoAction: false,
            },
            {
                name: 'money-and-orders',
                assignWhen:
                    'Receipts, invoices, confirmations, order and delivery updates, bank notices, and official financial documents when no unresolved problem requires action.',
                autoAction: false,
            },
            {
                name: 'opportunities',
                assignWhen:
                    'A specific, credible job, business, partnership, speaking, media, funding, or collaboration opportunity with relevant context. Excludes generic mass pitches.',
                autoAction: false,
            },
            {
                name: 'newsletters',
                assignWhen:
                    'Opt-in recurring editorial, product, industry, creator, or community content intended for later reading.',
                autoAction: false,
            },
            {
                name: 'routine-notifications',
                assignWhen:
                    'Low-risk automated social, application, monitoring, digest, or system notifications that require no action. Excludes security, access, payment, delivery, legal, and service-failure alerts.',
                autoAction: true,
            },
            {
                name: 'cold-outreach',
                assignWhen:
                    'Unsolicited commercial, recruiting, PR, SEO, agency, sponsorship, or link-exchange outreach with no relevant active relationship or conversation.',
                autoAction: true,
            },
            {
                name: 'junk',
                assignWhen:
                    'Phishing, scams, deceptive promotions, irrelevant mass spam, malicious requests, or incoherent disposable mail.',
                autoAction: true,
            },
            {
                name: 'review',
                assignWhen:
                    'A legitimate or ambiguous message that does not clearly fit another configured label.',
                autoAction: false,
            },
        ],
    },
    {
        id: 'founders',
        label: 'Founders, executives, and operators',
        rows: [
            {
                name: 'decision-required',
                assignWhen:
                    'A legitimate message that requires the recipient to approve, reject, choose, sign, prioritize, or make a business decision. Exclude FYI updates with no explicit decision.',
                autoAction: false,
            },
            {
                name: 'customer-risk',
                assignWhen:
                    'A customer or partner reports severe dissatisfaction, cancellation intent, failed delivery, material outage, payment dispute, or another issue that threatens revenue or trust.',
                autoAction: false,
            },
            {
                name: 'investor-and-board',
                assignWhen:
                    'Direct communication from current investors, board members, or prospective investors about fundraising, governance, performance, introductions, or company strategy. Exclude mass investor newsletters.',
                autoAction: false,
            },
            {
                name: 'team-blocker',
                assignWhen:
                    "A colleague cannot continue important work without the recipient's answer, access, approval, or intervention.",
                autoAction: false,
            },
            {
                name: 'delegatable',
                assignWhen:
                    'A legitimate operational request that needs action but can reasonably be handled by finance, legal, support, recruiting, or another team member instead of the recipient.',
                autoAction: false,
            },
            {
                name: 'vendor-pitch',
                assignWhen:
                    'Unsolicited vendor, agency, software, recruiting, PR, or consulting outreach with no active evaluation or existing relationship.',
                autoAction: false,
            },
            {
                name: 'review',
                assignWhen:
                    'A legitimate or ambiguous message that does not clearly match another founder and operator category.',
                autoAction: false,
            },
        ],
    },
    {
        id: 'sales',
        label: 'Sales and business development',
        rows: [
            {
                name: 'hot-lead',
                assignWhen:
                    'A prospect expresses concrete interest in buying: requests pricing, a demo, security information, a proposal, procurement steps, or mentions budget, authority, need, or timeline.',
                autoAction: false,
            },
            {
                name: 'deal-action',
                assignWhen:
                    'An active prospect or customer thread requires a reply, meeting follow-up, document, negotiation step, or internal commitment to keep the opportunity moving.',
                autoAction: false,
            },
            {
                name: 'partner-opportunity',
                assignWhen:
                    'A relevant proposal for integration, distribution, co-marketing, referral, reseller, marketplace, or strategic partnership with evidence of mutual fit.',
                autoAction: false,
            },
            {
                name: 'customer-success',
                assignWhen:
                    'A current customer asks for help, onboarding, expansion, renewal, product guidance, or reports a risk that may affect retention.',
                autoAction: false,
            },
            {
                name: 'sales-automation',
                assignWhen:
                    'CRM notifications, lead alerts, call summaries, sequence reports, and other automated sales operations messages that do not require a personal reply.',
                autoAction: false,
            },
            {
                name: 'irrelevant-outreach',
                assignWhen:
                    "An unsolicited offer selling tools or services to the sales team, rather than a prospect showing interest in the company's product.",
                autoAction: false,
            },
            {
                name: 'review',
                assignWhen:
                    'A legitimate or ambiguous message that does not clearly match another sales and business development category.',
                autoAction: false,
            },
        ],
    },
    {
        id: 'investors',
        label: 'Investors, venture capital, and angel networks',
        rows: [
            {
                name: 'founder-intro',
                assignWhen:
                    'A credible introduction to a founder or company, ideally from a known contact or with specific context explaining why the opportunity fits the investment thesis.',
                autoAction: false,
            },
            {
                name: 'active-deal',
                assignWhen:
                    'Communication about a company already under evaluation: diligence material, data room access, partner meeting, references, terms, allocation, or investment committee work.',
                autoAction: false,
            },
            {
                name: 'portfolio-action',
                assignWhen:
                    'A portfolio founder requests an introduction, hiring help, fundraising support, strategic advice, escalation, or another concrete action.',
                autoAction: false,
            },
            {
                name: 'lp-and-fund',
                assignWhen:
                    'Communication from limited partners, fund administrators, legal counsel, auditors, or service providers about fundraising, reporting, capital calls, compliance, or fund operations.',
                autoAction: false,
            },
            {
                name: 'ecosystem-update',
                assignWhen:
                    'Relevant market research, demo-day information, accelerator updates, or industry news worth reading but not requiring an immediate reply.',
                autoAction: false,
            },
            {
                name: 'mass-fundraising-pitch',
                assignWhen:
                    'Generic or automated fundraising outreach with little personalization, unclear thesis fit, or no credible introduction.',
                autoAction: false,
            },
            {
                name: 'review',
                assignWhen:
                    'A legitimate or ambiguous message that does not clearly match another investor and fund category.',
                autoAction: false,
            },
        ],
    },
    {
        id: 'recruiting',
        label: 'Recruiting and people operations',
        rows: [
            {
                name: 'candidate-action',
                assignWhen:
                    'A candidate asks a substantive question, provides requested information, or needs a reply to continue an active hiring process.',
                autoAction: false,
            },
            {
                name: 'interview-scheduling',
                assignWhen:
                    'Availability, confirmation, rescheduling, interviewer coordination, or logistics for an interview or hiring conversation.',
                autoAction: false,
            },
            {
                name: 'offer-and-closing',
                assignWhen:
                    'Offer discussion, compensation questions, references, background checks, start dates, negotiation, or another late-stage hiring step.',
                autoAction: false,
            },
            {
                name: 'employee-sensitive',
                assignWhen:
                    'A personal, confidential, workplace, performance, wellbeing, grievance, or policy matter that requires careful human review. Never archive automatically.',
                autoAction: false,
            },
            {
                name: 'ats-automation',
                assignWhen:
                    'Automated application receipts, status notifications, interview reminders, or recruiting system reports with no manual response required.',
                autoAction: false,
            },
            {
                name: 'recruiting-vendor-pitch',
                assignWhen:
                    'Unsolicited staffing agency, job board, sourcing tool, employer-branding, or recruiting software sales outreach.',
                autoAction: false,
            },
            {
                name: 'review',
                assignWhen:
                    'A legitimate or ambiguous message that does not clearly match another recruiting and people operations category.',
                autoAction: false,
            },
        ],
    },
    {
        id: 'freelancers',
        label: 'Freelancers, consultants, and creators',
        rows: [
            {
                name: 'qualified-opportunity',
                assignWhen:
                    'A specific paid project, sponsorship, collaboration, or consulting request with relevant scope, timing, budget signals, and a credible sender.',
                autoAction: false,
            },
            {
                name: 'client-action',
                assignWhen:
                    'A current client asks for a deliverable, decision, revision, meeting, approval, or information needed to advance active work.',
                autoAction: false,
            },
            {
                name: 'payment-and-contract',
                assignWhen:
                    'Contracts, statements of work, invoices, payment confirmations, tax forms, procurement, or legal terms related to paid work.',
                autoAction: false,
            },
            {
                name: 'audience-and-community',
                assignWhen:
                    'Meaningful messages from readers, viewers, community members, or peers that contain feedback, questions, or relationship value.',
                autoAction: false,
            },
            {
                name: 'platform-update',
                assignWhen:
                    'Routine notifications from publishing, analytics, commerce, booking, or creator platforms with no immediate action required.',
                autoAction: false,
            },
            {
                name: 'low-quality-collab',
                assignWhen:
                    'Generic gifting, affiliate, guest-post, backlink, exposure-only, or mass sponsorship outreach without relevant terms or genuine personalization.',
                autoAction: false,
            },
            {
                name: 'review',
                assignWhen:
                    'A legitimate or ambiguous message that does not clearly match another freelancer, consultant, or creator category.',
                autoAction: false,
            },
        ],
    },
    {
        id: 'support',
        label: 'Customer support and e-commerce',
        rows: [
            {
                name: 'urgent-escalation',
                assignWhen:
                    'A safety, security, fraud, legal, account-access, widespread outage, or severe customer-impact issue requiring immediate human review.',
                autoAction: false,
            },
            {
                name: 'refund-or-billing',
                assignWhen:
                    'A customer asks about a charge, invoice, refund, duplicate payment, failed payment, cancellation, or subscription billing.',
                autoAction: false,
            },
            {
                name: 'delivery-or-order',
                assignWhen:
                    'Order status, delayed or missing delivery, damaged goods, wrong items, returns, exchanges, or shipping address problems.',
                autoAction: false,
            },
            {
                name: 'product-help',
                assignWhen:
                    'A customer needs instructions, troubleshooting, compatibility guidance, onboarding, or help using the product.',
                autoAction: false,
            },
            {
                name: 'feedback-and-feature',
                assignWhen:
                    'Product feedback, feature requests, reviews, survey responses, or suggestions that do not require urgent support.',
                autoAction: false,
            },
            {
                name: 'automated-system-mail',
                assignWhen:
                    'Monitoring reports, ticket acknowledgements, routine platform notifications, and other automated operational email with no direct customer request.',
                autoAction: false,
            },
            {
                name: 'review',
                assignWhen:
                    'A legitimate or ambiguous message that does not clearly match another support and e-commerce category.',
                autoAction: false,
            },
        ],
    },
]
