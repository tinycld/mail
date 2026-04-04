import { execSync } from 'node:child_process'
import * as fs from 'node:fs'
import * as path from 'node:path'

const ROOT = path.resolve(import.meta.dirname, '..')
const GENERATED_DIR = path.join(ROOT, 'lib/generated')
const ROUTES_BASE = path.join(ROOT, 'app/app')
const MIGRATIONS_DIR = path.join(ROOT, 'server/pb_migrations')
const HOOKS_DIR = path.join(ROOT, 'server/pb_hooks')
const SERVER_DIR = path.join(ROOT, 'server')
const LINKS_MANIFEST = path.join(ROOT, '.addon-links.json')

interface AddonManifest {
    name: string
    slug: string
    version: string
    description: string
    routes: { directory: string }
    nav: { label: string; icon: string; order?: number }
    migrations?: { directory: string }
    hooks?: { directory: string }
    collections?: { register: string; types: string }
    sidebar?: { component: string }
    settings?: { slug: string; component: string; label: string }[]
    seed?: { script: string }
    tests?: { directory: string }
    server?: { package: string; module: string }
    dependencies?: string[]
}

interface LinksManifest {
    symlinks: string[]
    generatedFiles: string[]
}

function loadPreviousLinks(): LinksManifest {
    try {
        return JSON.parse(fs.readFileSync(LINKS_MANIFEST, 'utf-8'))
    } catch {
        return { symlinks: [], generatedFiles: [] }
    }
}

function cleanPrevious(manifest: LinksManifest) {
    for (const filePath of [...manifest.symlinks, ...manifest.generatedFiles]) {
        try {
            const stat = fs.lstatSync(filePath)
            if (stat.isSymbolicLink() || stat.isFile()) {
                fs.unlinkSync(filePath)
            }
        } catch {
            // already gone
        }
    }

    // Clean empty generated route directories
    const routeDirs = new Set(
        manifest.generatedFiles.filter(f => f.startsWith(ROUTES_BASE)).map(f => path.dirname(f))
    )
    for (const dir of Array.from(routeDirs).sort((a, b) => b.length - a.length)) {
        try {
            const entries = fs.readdirSync(dir)
            if (entries.length === 0) {
                fs.rmdirSync(dir)
            }
        } catch {
            // directory already gone
        }
    }
}

function resolvePackageDir(packageName: string): string {
    const resolved = import.meta.resolve?.(`${packageName}/package.json`)
    if (resolved) {
        const url = new URL(resolved)
        return path.dirname(url.pathname)
    }
    // Fallback: check common locations
    const workspacePath = path.join(ROOT, 'packages', packageName.replace(/^@tinycld\//, ''))
    if (fs.existsSync(workspacePath)) {
        return workspacePath
    }
    const nodeModulesPath = path.join(ROOT, 'node_modules', packageName)
    if (fs.existsSync(nodeModulesPath)) {
        return nodeModulesPath
    }
    throw new Error(`Cannot resolve package directory for ${packageName}`)
}

function loadManifest(packageDir: string): AddonManifest {
    for (const ext of ['ts', 'js']) {
        const manifestPath = path.join(packageDir, `manifest.${ext}`)
        if (!fs.existsSync(manifestPath)) continue
        const content = fs.readFileSync(manifestPath, 'utf-8')

        // Match either `export default { ... }` or `const/let/var X = { ... }`
        const match =
            content.match(/(?:export\s+default|module\.exports\s*=)\s*(\{[\s\S]*\})/) ??
            content.match(/(?:const|let|var)\s+\w+\s*=\s*(\{[\s\S]*\})\s*(?:;?\s*$)/m)
        if (match) {
            const obj = new Function(`return (${match[1]})`)()
            return obj as AddonManifest
        }
    }
    throw new Error(`No manifest found in ${packageDir}`)
}

function walkFiles(dir: string, base = ''): string[] {
    if (!fs.existsSync(dir)) return []
    const results: string[] = []
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
        const rel = path.join(base, entry.name)
        if (entry.isDirectory()) {
            results.push(...walkFiles(path.join(dir, entry.name), rel))
        } else {
            results.push(rel)
        }
    }
    return results
}

function generateRoutes(
    packageName: string,
    manifest: AddonManifest,
    packageDir: string
): string[] {
    const screensDir = path.join(packageDir, manifest.routes.directory)
    const files = walkFiles(screensDir)
    const generated: string[] = []

    const addonRouteDir = path.join(ROUTES_BASE, manifest.slug)
    fs.mkdirSync(addonRouteDir, { recursive: true })

    for (const file of files) {
        const ext = path.extname(file)
        if (!['.tsx', '.ts', '.jsx', '.js'].includes(ext)) continue

        const withoutExt = file.replace(/\.[^.]+$/, '')
        const importPath = `${packageName}/${manifest.routes.directory}/${withoutExt}`
        const outFile = path.join(addonRouteDir, file)

        fs.mkdirSync(path.dirname(outFile), { recursive: true })
        fs.writeFileSync(outFile, `export { default } from '${importPath}'\n`)
        generated.push(outFile)
    }

    return generated
}

function symlinkOrFileExists(p: string): boolean {
    try {
        fs.lstatSync(p)
        return true
    } catch {
        return false
    }
}

function createSymlinks(manifest: AddonManifest, packageDir: string): string[] {
    const created: string[] = []

    if (manifest.migrations?.directory) {
        const migrationsSource = path.join(packageDir, manifest.migrations.directory)
        if (fs.existsSync(migrationsSource)) {
            for (const file of fs.readdirSync(migrationsSource)) {
                const target = path.join(MIGRATIONS_DIR, file)
                const source = path.join(migrationsSource, file)
                if (!symlinkOrFileExists(target)) {
                    fs.symlinkSync(source, target)
                    created.push(target)
                }
            }
        }
    }

    if (manifest.hooks?.directory) {
        const hooksSource = path.join(packageDir, manifest.hooks.directory)
        if (fs.existsSync(hooksSource)) {
            fs.mkdirSync(HOOKS_DIR, { recursive: true })
            for (const file of fs.readdirSync(hooksSource)) {
                const target = path.join(HOOKS_DIR, file)
                const source = path.join(hooksSource, file)
                if (!symlinkOrFileExists(target)) {
                    fs.symlinkSync(source, target)
                    created.push(target)
                }
            }
        }
    }

    return created
}

function slugToIdentifier(slug: string): string {
    return slug.replace(/-([a-z])/g, (_, c) => c.toUpperCase())
}

function slugToPascal(slug: string): string {
    return slug
        .split('-')
        .map(s => s.charAt(0).toUpperCase() + s.slice(1))
        .join('')
}

function generateCollectionsFile(
    addonsInfo: { packageName: string; manifest: AddonManifest }[]
): string {
    const withCollections = addonsInfo.filter(a => a.manifest.collections)

    if (withCollections.length === 0) {
        return [
            '// Auto-generated by scripts/generate-addons.ts — do not edit',
            "import type { Schema } from '~/types/pbSchema'",
            "import type { createCollection } from 'pbtsdb'",
            "import type { CoreStores } from '~/lib/pocketbase'",
            '',
            'export type MergedSchema = Schema',
            'type NewCollection = ReturnType<typeof createCollection<MergedSchema>>',
            '',
            'export function addonStores(_newCollection: NewCollection, _coreStores: CoreStores) {',
            '    return {}',
            '}',
            '',
        ].join('\n')
    }

    const schemaImports = withCollections.map(a => {
        const pascal = slugToPascal(a.manifest.slug)
        return `import type { ${pascal}Schema } from '${a.packageName}/${a.manifest.collections?.types}'`
    })

    const registerImports = withCollections.map(a => {
        const id = slugToIdentifier(a.manifest.slug)
        return `import { registerCollections as ${id}Register } from '${a.packageName}/${a.manifest.collections?.register}'`
    })

    const schemaUnion = withCollections
        .map(a => `${slugToPascal(a.manifest.slug)}Schema`)
        .join(' & ')

    const spreads = withCollections.map(a => {
        const id = slugToIdentifier(a.manifest.slug)
        return `        ...${id}Register(newCollection, coreStores),`
    })

    return [
        '// Auto-generated by scripts/generate-addons.ts — do not edit',
        "import type { Schema } from '~/types/pbSchema'",
        "import type { createCollection } from 'pbtsdb'",
        "import type { CoreStores } from '~/lib/pocketbase'",
        ...schemaImports,
        ...registerImports,
        '',
        `export type MergedSchema = Schema & ${schemaUnion}`,
        'type NewCollection = ReturnType<typeof createCollection<MergedSchema>>',
        '',
        'export function addonStores(newCollection: NewCollection, coreStores: CoreStores) {',
        '    return {',
        ...spreads,
        '    }',
        '}',
        '',
    ].join('\n')
}

function generateRegistryFile(
    addonsInfo: { packageName: string; manifest: AddonManifest }[]
): string {
    if (addonsInfo.length === 0) {
        return [
            '// Auto-generated by scripts/generate-addons.ts — do not edit',
            "import type { AddonManifest } from '~/lib/addons/types'",
            '',
            'export const addonRegistry: (AddonManifest & { packageName: string })[] = []',
            '',
        ].join('\n')
    }

    const imports = addonsInfo.map(a => {
        const id = slugToIdentifier(a.manifest.slug)
        return `import ${id}Manifest from '${a.packageName}/manifest'`
    })

    const entries = addonsInfo.map(a => {
        const id = slugToIdentifier(a.manifest.slug)
        return `    { ...${id}Manifest, packageName: '${a.packageName}' },`
    })

    return [
        '// Auto-generated by scripts/generate-addons.ts — do not edit',
        ...imports,
        '',
        'export const addonRegistry = [',
        ...entries,
        ']',
        '',
    ].join('\n')
}

function generateSidebarsFile(
    addonsInfo: { packageName: string; manifest: AddonManifest }[]
): string {
    const withSidebars = addonsInfo.filter(a => a.manifest.sidebar?.component)

    const imports = withSidebars.map(a => {
        const id = slugToIdentifier(a.manifest.slug)
        return `import ${id}Sidebar from '${a.packageName}/${a.manifest.sidebar?.component}'`
    })

    const entries = addonsInfo.map(a => {
        if (a.manifest.sidebar?.component) {
            const id = slugToIdentifier(a.manifest.slug)
            return `    '${a.manifest.slug}': ${id}Sidebar,`
        }
        return `    '${a.manifest.slug}': null,`
    })

    return [
        '// Auto-generated by scripts/generate-addons.ts — do not edit',
        "import type { ComponentType } from 'react'",
        ...imports,
        '',
        'interface AddonSidebarProps {',
        '    basePath: string',
        '    isCollapsed: boolean',
        '}',
        '',
        'export const addonSidebars: Record<string, ComponentType<AddonSidebarProps> | null> = {',
        ...entries,
        '}',
        '',
    ].join('\n')
}

function generateSettingsFile(
    addonsInfo: { packageName: string; manifest: AddonManifest }[]
): string {
    const withSettings = addonsInfo.filter(
        a => a.manifest.settings && a.manifest.settings.length > 0
    )

    if (withSettings.length === 0) {
        return [
            '// Auto-generated by scripts/generate-addons.ts — do not edit',
            "import type { ComponentType } from 'react'",
            '',
            'export interface AddonSettingsPanel {',
            '    slug: string',
            '    label: string',
            '    Component: ComponentType',
            '}',
            '',
            'export interface AddonSettingsGroup {',
            '    addonName: string',
            '    addonSlug: string',
            '    panels: AddonSettingsPanel[]',
            '}',
            '',
            'export const addonSettings: AddonSettingsGroup[] = []',
            '',
        ].join('\n')
    }

    const imports: string[] = []
    const groups: string[] = []

    for (const a of withSettings) {
        const panels: string[] = []
        for (const panel of a.manifest.settings ?? []) {
            const id = `${slugToIdentifier(a.manifest.slug)}${slugToPascal(panel.slug)}`
            imports.push(`import ${id} from '${a.packageName}/${panel.component}'`)
            panels.push(
                `            { slug: '${panel.slug}', label: '${panel.label}', Component: ${id} },`
            )
        }
        groups.push(
            [
                '    {',
                `        addonName: '${a.manifest.name}',`,
                `        addonSlug: '${a.manifest.slug}',`,
                '        panels: [',
                ...panels,
                '        ],',
                '    },',
            ].join('\n')
        )
    }

    return [
        '// Auto-generated by scripts/generate-addons.ts — do not edit',
        "import type { ComponentType } from 'react'",
        ...imports,
        '',
        'export interface AddonSettingsPanel {',
        '    slug: string',
        '    label: string',
        '    Component: ComponentType',
        '}',
        '',
        'export interface AddonSettingsGroup {',
        '    addonName: string',
        '    addonSlug: string',
        '    panels: AddonSettingsPanel[]',
        '}',
        '',
        'export const addonSettings: AddonSettingsGroup[] = [',
        ...groups,
        ']',
        '',
    ].join('\n')
}

function generateSeedsFile(addonsInfo: { packageName: string; manifest: AddonManifest }[]): string {
    const withSeeds = addonsInfo.filter(a => a.manifest.seed?.script)

    if (withSeeds.length === 0) {
        return [
            '// Auto-generated by scripts/generate-addons.ts — do not edit',
            "import type PocketBase from 'pocketbase'",
            '',
            'export interface SeedContext {',
            '    user: { id: string }',
            '    org: { id: string }',
            '    userOrg: { id: string }',
            '}',
            '',
            'export type AddonSeedFn = (pb: PocketBase, context: SeedContext) => Promise<void>',
            '',
            'export const addonSeeds: Record<string, AddonSeedFn> = {}',
            '',
        ].join('\n')
    }

    const imports = withSeeds.map(a => {
        const id = slugToIdentifier(a.manifest.slug)
        return `import ${id}Seed from '${a.packageName}/${a.manifest.seed?.script}'`
    })

    const entries = withSeeds.map(a => {
        const id = slugToIdentifier(a.manifest.slug)
        return `    '${a.manifest.slug}': ${id}Seed,`
    })

    return [
        '// Auto-generated by scripts/generate-addons.ts — do not edit',
        "import type PocketBase from 'pocketbase'",
        ...imports,
        '',
        'export interface SeedContext {',
        '    user: { id: string }',
        '    org: { id: string }',
        '    userOrg: { id: string }',
        '}',
        '',
        'export type AddonSeedFn = (pb: PocketBase, context: SeedContext) => Promise<void>',
        '',
        'export const addonSeeds: Record<string, AddonSeedFn> = {',
        ...entries,
        '}',
        '',
    ].join('\n')
}

const GO_MOD_MARKER_START = '// --- addon extensions (auto-generated, do not edit) ---'
const GO_MOD_MARKER_END = '// --- end addon extensions ---'

function generateAddonExtensionsGo(
    addonsInfo: { packageName: string; manifest: AddonManifest; packageDir: string }[]
): string {
    const withServer = addonsInfo.filter(
        a =>
            a.manifest.server?.package &&
            fs.existsSync(path.join(a.packageDir, a.manifest.server.package))
    )

    if (withServer.length === 0) {
        return [
            '// Code generated by scripts/generate-addons.ts. DO NOT EDIT.',
            'package main',
            '',
            'import "github.com/pocketbase/pocketbase"',
            '',
            'func registerAddonExtensions(_ *pocketbase.PocketBase) {}',
            '',
        ].join('\n')
    }

    const imports = withServer.map(a => {
        const id = slugToIdentifier(a.manifest.slug)
        return `\t${id} "${a.manifest.server?.module}"`
    })

    const calls = withServer.map(a => {
        const id = slugToIdentifier(a.manifest.slug)
        return `\t${id}.Register(app)`
    })

    return [
        '// Code generated by scripts/generate-addons.ts. DO NOT EDIT.',
        'package main',
        '',
        'import (',
        '\t"github.com/pocketbase/pocketbase"',
        ...imports,
        ')',
        '',
        'func registerAddonExtensions(app *pocketbase.PocketBase) {',
        ...calls,
        '}',
        '',
    ].join('\n')
}

function updateGoMod(
    addonsInfo: { packageName: string; manifest: AddonManifest; packageDir: string }[]
) {
    const withServer = addonsInfo.filter(
        a =>
            a.manifest.server?.package &&
            fs.existsSync(path.join(a.packageDir, a.manifest.server.package))
    )

    const goModPath = path.join(SERVER_DIR, 'go.mod')
    let content = fs.readFileSync(goModPath, 'utf-8')

    // Strip existing addon block
    const startIdx = content.indexOf(GO_MOD_MARKER_START)
    const endIdx = content.indexOf(GO_MOD_MARKER_END)
    if (startIdx !== -1 && endIdx !== -1) {
        content =
            content.slice(0, startIdx).trimEnd() +
            '\n' +
            content.slice(endIdx + GO_MOD_MARKER_END.length).trimStart()
    }

    // Remove trailing whitespace/newlines and ensure single trailing newline
    content = `${content.trimEnd()}\n`

    if (withServer.length > 0) {
        const lines = [
            '',
            GO_MOD_MARKER_START,
            ...withServer.map(a => `require ${a.manifest.server?.module} v0.0.0`),
            '',
            ...withServer.map(a => {
                const relPath = path.relative(
                    SERVER_DIR,
                    path.join(a.packageDir, a.manifest.server?.package ?? '')
                )
                return `replace ${a.manifest.server?.module} => ${relPath}`
            }),
            GO_MOD_MARKER_END,
            '',
        ]
        content += lines.join('\n')
    }

    fs.writeFileSync(goModPath, content)
}

function runGoModTidy() {
    try {
        execSync('go mod tidy', { cwd: SERVER_DIR, stdio: 'inherit' })
    } catch {
        // go may not be available (e.g. Docker Node-only stage)
    }
}

async function main() {
    // Load addon config
    const configPath = path.join(ROOT, 'tinycld.addons.ts')
    const configContent = fs.readFileSync(configPath, 'utf-8')
    const match = configContent.match(/\[([^\]]*)\]/)
    if (!match) {
        process.exit(1)
    }

    const addonPackages = match[1]
        .split(',')
        .map(s => s.trim().replace(/['"]/g, ''))
        .filter(Boolean)

    // Clean previous generated files
    const previousLinks = loadPreviousLinks()
    cleanPrevious(previousLinks)

    // Ensure output dirs
    fs.mkdirSync(GENERATED_DIR, { recursive: true })
    fs.mkdirSync(ROUTES_BASE, { recursive: true })

    const allSymlinks: string[] = []
    const allGenerated: string[] = []
    const addonsInfo: { packageName: string; manifest: AddonManifest; packageDir: string }[] = []

    for (const packageName of addonPackages) {
        const packageDir = resolvePackageDir(packageName)
        const manifest = loadManifest(packageDir)
        addonsInfo.push({ packageName, manifest, packageDir })

        // Generate routes
        const routeFiles = generateRoutes(packageName, manifest, packageDir)
        allGenerated.push(...routeFiles)

        // Create symlinks for migrations and hooks
        const links = createSymlinks(manifest, packageDir)
        allSymlinks.push(...links)
    }

    // Generate collection registration file
    const collectionsFile = path.join(GENERATED_DIR, 'addon-collections.ts')
    fs.writeFileSync(collectionsFile, generateCollectionsFile(addonsInfo))
    allGenerated.push(collectionsFile)

    // Generate registry file
    const registryFile = path.join(GENERATED_DIR, 'addon-registry.ts')
    fs.writeFileSync(registryFile, generateRegistryFile(addonsInfo))
    allGenerated.push(registryFile)

    // Generate sidebars file
    const sidebarsFile = path.join(GENERATED_DIR, 'addon-sidebars.ts')
    fs.writeFileSync(sidebarsFile, generateSidebarsFile(addonsInfo))
    allGenerated.push(sidebarsFile)

    // Generate settings file
    const settingsFile = path.join(GENERATED_DIR, 'addon-settings.ts')
    fs.writeFileSync(settingsFile, generateSettingsFile(addonsInfo))
    allGenerated.push(settingsFile)

    // Generate seeds file
    const seedsFile = path.join(GENERATED_DIR, 'addon-seeds.ts')
    fs.writeFileSync(seedsFile, generateSeedsFile(addonsInfo))
    allGenerated.push(seedsFile)

    // Generate Go server extension file
    const addonExtensionsFile = path.join(SERVER_DIR, 'addon_extensions.go')
    fs.writeFileSync(addonExtensionsFile, generateAddonExtensionsGo(addonsInfo))
    allGenerated.push(addonExtensionsFile)

    // Update server/go.mod with addon require/replace directives
    updateGoMod(addonsInfo)
    runGoModTidy()

    // Save manifest for cleanup
    const linksManifest: LinksManifest = {
        symlinks: allSymlinks,
        generatedFiles: allGenerated,
    }
    fs.writeFileSync(LINKS_MANIFEST, JSON.stringify(linksManifest, null, 2))
}

main()
