const manifest = {
    name: 'Mail',
    slug: 'mail',
    version: '0.2.1',
    description: 'Gmail-style email client for your organization',
    routes: { directory: 'screens' },
    nav: { label: 'Mail', icon: 'mail', order: 5, shortcut: 'm' },
    sidebar: { component: 'sidebar' },
    slots: ['sidebar.after-labels'],
    settings: [
        { slug: 'provider', label: 'Provider', component: 'settings/provider' },
        { slug: 'mailboxes', label: 'Mailboxes', component: 'settings/mailboxes' },
    ],
    systemSettings: [
        { slug: 'provider', label: 'Provider', component: 'system-settings/provider' },
    ],
    migrations: { directory: 'pb-migrations' },
    collections: { register: 'collections', types: 'types' },
    help: { directory: 'help' },
    seed: { script: 'seed' },
    server: { package: 'server', module: 'tinycld.org/packages/mail' },
    repository: { url: 'https://github.com/tinycld/mail' },
    peerVersions: { '@tinycld/core': '>=0.0.4 <0.1.0' },
}

export default manifest
