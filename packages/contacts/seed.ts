import type PocketBase from 'pocketbase'

interface SeedContext {
    userOrg: { id: string }
}

const SAMPLE_CONTACTS = [
    {
        first_name: 'Alice',
        last_name: 'Johnson',
        email: 'alice.johnson@example.com',
        phone: '(555) 123-4567',
        company: 'Acme Corp',
        job_title: 'Product Manager',
        favorite: true,
        notes: 'Met at the annual conference.',
    },
    {
        first_name: 'Bob',
        last_name: 'Smith',
        email: 'bob.smith@example.com',
        phone: '(555) 234-5678',
        company: 'Globex Inc',
        job_title: 'Software Engineer',
        favorite: false,
        notes: '',
    },
    {
        first_name: 'Carol',
        last_name: 'Williams',
        email: 'carol.w@example.com',
        phone: '(555) 345-6789',
        company: 'Initech',
        job_title: 'VP of Sales',
        favorite: true,
        notes: 'Key partner contact.',
    },
    {
        first_name: 'David',
        last_name: 'Brown',
        email: 'david.brown@example.com',
        phone: '',
        company: 'Umbrella LLC',
        job_title: 'Designer',
        favorite: false,
        notes: '',
    },
    {
        first_name: 'Eva',
        last_name: 'Martinez',
        email: 'eva.m@example.com',
        phone: '(555) 567-8901',
        company: 'Acme Corp',
        job_title: 'CTO',
        favorite: false,
        notes: 'Introduced by Alice.',
    },
    {
        first_name: 'Frank',
        last_name: 'Lee',
        email: 'frank.lee@example.com',
        phone: '(555) 678-9012',
        company: '',
        job_title: 'Freelance Consultant',
        favorite: true,
        notes: '',
    },
    {
        first_name: 'Grace',
        last_name: 'Kim',
        email: 'grace.kim@example.com',
        phone: '(555) 789-0123',
        company: 'Stark Industries',
        job_title: 'Marketing Director',
        favorite: false,
        notes: '',
    },
]

export default async function seed(pb: PocketBase, { userOrg }: SeedContext) {
    console.log('  Seeding contacts...')
    for (const contact of SAMPLE_CONTACTS) {
        await pb.collection('contacts').create({
            ...contact,
            owner: userOrg.id,
        })
    }
    console.log(`  Created ${SAMPLE_CONTACTS.length} sample contacts`)
}
