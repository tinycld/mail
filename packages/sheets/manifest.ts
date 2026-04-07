const manifest = {
    name: 'Sheets',
    slug: 'sheets',
    version: '0.1.0',
    description: 'Collaborative spreadsheets for your organization',
    routes: { directory: 'screens' },
    nav: { label: 'Sheets', icon: 'table', order: 13 },
    sidebar: { component: 'sidebar' },
    provider: { component: 'provider' },
    migrations: { directory: 'pb-migrations' },
    collections: { register: 'collections', types: 'types' },
    seed: { script: 'seed' },
}

export default manifest
