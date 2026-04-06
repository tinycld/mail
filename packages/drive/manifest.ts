const manifest = {
    name: 'Drive',
    slug: 'drive',
    version: '0.1.0',
    description: 'Cloud file storage for your organization',
    routes: { directory: 'screens' },
    nav: { label: 'Drive', icon: 'hard-drive', order: 12 },
    sidebar: { component: 'sidebar' },
    provider: { component: 'provider' },
}

export default manifest
