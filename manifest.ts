const manifest = {
    name: 'Mail',
    slug: 'mail',
    version: '0.6.1',
    description: 'Gmail-style email for your server',
    routes: { directory: 'screens' },
    nav: { label: 'Mail', icon: 'mail', order: 5, shortcut: 'm' },
    sidebar: { component: 'sidebar' },
    slots: ['sidebar.after-labels', 'setup-domain-options'],
    setupSteps: [
        {
            id: 'email-domain',
            label: 'Domain',
            module: 'setup/EmailDomainStep',
            order: 'a2V',
        },
        { id: 'address', label: 'Address', module: 'setup/AddressStep', order: 'a2k' },
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
    server: { package: 'server', module: 'tinycld.org/packages/mail' },
    // The supervisor that holds the public ports binds these once and hands
    // each one to whichever server child is current, by name; mail's own-ports path
    // (registerMailListeners) asks for them by the same names before it
    // binds anything itself. addrEnv/enabled mirror the env vars
    // server/imap_server.go, server/smtp_server.go and
    // server/smtp_inbound_server.go already read.
    ports: [
        {
            name: 'imaps',
            port: 993,
            addrEnv: 'IMAPS_ADDR',
            enabled: { env: 'IMAP_ENABLED', default: true },
        },
        {
            name: 'submissions',
            port: 465,
            addrEnv: 'SMTPS_ADDR',
            enabled: { env: 'SMTP_ENABLED', default: true },
        },
        {
            name: 'smtp',
            port: 25,
            addrEnv: 'SMTP_INBOUND_ADDR',
            enabled: { env: 'MAIL_INBOUND_SMTP_ENABLED', default: false },
        },
    ],
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
    peerVersions: { '@tinycld/core': '>=0.6.2 <0.7.0' },
}

export default manifest
