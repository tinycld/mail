const manifest = {
    name: 'Mail',
    slug: 'mail',
    version: '0.5.0',
    description: 'Gmail-style email for your server',
    routes: { directory: 'screens' },
    nav: { label: 'Mail', icon: 'mail', order: 5, shortcut: 'm' },
    sidebar: { component: 'sidebar' },
    slots: ['sidebar.after-labels', 'setup-domain-options'],
    setupSteps: [
        {
            id: 'email-domain',
            label: 'Email domain',
            module: 'setup/EmailDomainStep',
            order: 'a2V',
        },
        { id: 'address', label: 'Your address', module: 'setup/AddressStep', order: 'a2k' },
    ],
    settings: [
        // 'Domains' — matches the screen's own heading, and keeps this
        // distinct from the systemSettings 'Provider' panel below, which is
        // where the provider itself is chosen.
        { slug: 'provider', label: 'Domains', component: 'settings/provider' },
        { slug: 'mailboxes', label: 'Mailboxes', component: 'settings/mailboxes' },
    ],
    // keyPrefix: these are deployment-wide credentials. Where a service provider
    // owns them, this panel is hidden — it could not save there, because the
    // values live in the provider's memory and never reach this deployment's
    // database. The org-scoped 'provider' panel above (Domains) is unaffected
    // and stays editable everywhere.
    systemSettings: [
        {
            slug: 'provider',
            label: 'Provider',
            component: 'system-settings/provider',
            keyPrefix: 'mail.',
        },
    ],
    migrations: { directory: 'pb-migrations' },
    collections: { register: 'collections', types: 'types' },
    help: { directory: 'help' },
    seed: { script: 'seed' },
    // Mail is searchable through core's federated /api/search, which reads the
    // Go source registered in server/. The in-app advanced search keeps its own
    // /api/mail/search route — that is a different feature, with structured
    // filters the palette does not offer.
    search: { adapter: 'search-adapter' },
    automation: { definitions: 'automation' },
    // Message bodies are real disk. No ownerField: a mailbox is shared by its
    // members, so these bytes count toward the deployment-wide ceiling only.
    quota: [{ collection: 'mail_messages', sizeField: 'total_size' }],
    // mailListeners: asks a supervisor that owns the public ports to hand
    // this package its mail listeners; Register discovers them at boot, and
    // under such a supervisor the package never binds a port itself.
    server: { package: 'server', module: 'tinycld.org/packages/mail', mailListeners: true },
    // The API payload contract (server/api) generated into
    // @tinycld/app-generated/mail-api — hooks import those types, so the
    // peerVersions floor below must stay >= the core that ships the emitter.
    payloads: { package: 'server/api' },
    // `tinycld mail ...` commands, compiled into the per-org CLI binary by
    // gen-cli.ts. The OAuth scopes the commands need are registered by
    // server/oauth_scopes.go, never declared here.
    // Cobra is the source of truth for the command list and --help.
    cli: {
        package: 'cli',
        module: 'tinycld.org/packages/mail/cli',
    },
    repository: { url: 'https://github.com/tinycld/mail' },
    peerVersions: { '@tinycld/core': '>=0.5.3 <0.6.0' },
}

export default manifest
