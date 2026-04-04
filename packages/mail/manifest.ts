const manifest = {
    name: 'Mail',
    slug: 'mail',
    version: '0.1.0',
    description: 'Gmail-style email client for your organization',
    routes: { directory: 'screens' },
    nav: { label: 'Mail', icon: 'mail', order: 5 },
    sidebar: { component: 'sidebar' },
    migrations: { directory: 'pb-migrations' },
    collections: { register: 'collections', types: 'types' },
    seed: { script: 'seed' },
    server: { package: 'server', module: 'tinycld/addons/mail' },
}

export default manifest
