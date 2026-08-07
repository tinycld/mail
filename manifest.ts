const manifest = {
    name: 'Mail',
    slug: 'mail',
    version: '0.3.1',
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
    // Message bodies are real disk. No ownerField: a mailbox is shared by its
    // members, so these bytes count toward the ORG ceiling only.
    quota: [{ collection: 'mail_messages', sizeField: 'total_size' }],
    // mailListeners: tells the multi-org router to create per-org mail
    // sockets; Register discovers them via coreserver.GetTenantContext —
    // the router owns every port, a tenant never binds one.
    server: { package: 'server', module: 'tinycld.org/packages/mail', mailListeners: true },
    // The API payload contract (server/api) generated into
    // @tinycld/app-generated/mail-api — hooks import those types, so the
    // peerVersions floor below must stay >= the core that ships the emitter.
    payloads: { package: 'server/api' },
    // `tinycld mail ...` commands, compiled into the per-org CLI binary by
    // gen-cli.ts. commands is display metadata (Cobra owns --help); scopes
    // feed the OAuth scope registry and consent screen.
    cli: {
        package: 'cli',
        module: 'tinycld.org/packages/mail/cli',
        commands: [
            { name: 'search', summary: 'Search messages' },
            { name: 'list', summary: 'List threads in a folder' },
            { name: 'read', summary: 'Print a message or thread' },
            { name: 'attachments', summary: 'List a message’s attachments' },
            { name: 'download', summary: 'Download attachments' },
            { name: 'send', summary: 'Send a message' },
            { name: 'mailboxes', summary: 'List mailboxes' },
            { name: 'status', summary: 'Per-folder unread counts' },
        ],
        scopes: ['mail:read', 'mail:send'],
    },
    repository: { url: 'https://github.com/tinycld/mail' },
    peerVersions: { '@tinycld/core': '>=0.0.5 <0.1.0' },
}

export default manifest
