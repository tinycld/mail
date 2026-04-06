const manifest = {
    name: 'Calendar',
    slug: 'calendar',
    version: '0.1.0',
    description: 'Shared calendar for your organization',
    routes: { directory: 'screens' },
    nav: { label: 'Calendar', icon: 'calendar', order: 8 },
    sidebar: { component: 'sidebar' },
    provider: { component: 'provider' },
    migrations: { directory: 'pb-migrations' },
    collections: { register: 'collections', types: 'types' },
    seed: { script: 'seed' },
    server: { package: 'server', module: 'tinycld/addons/calendar' },
}

export default manifest
