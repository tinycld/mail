const manifest = {
    name: 'Contacts',
    slug: 'contacts',
    version: '0.1.0',
    description: 'Shared contacts for your organization',
    routes: { directory: 'screens' },
    nav: { label: 'Contacts', icon: 'users', order: 10 },
    migrations: { directory: 'pb-migrations' },
    collections: { register: 'collections', types: 'types' },
    sidebar: { component: 'sidebar' },
    seed: { script: 'seed' },
    tests: { directory: 'tests' },
    server: { package: 'server', module: 'tinycld/addons/contacts' },
}

export default manifest
