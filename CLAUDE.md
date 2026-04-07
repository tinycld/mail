# Project Guidelines

## Overview
TinyCld is an One Stack React Native application backed by PocketBase . The repository lives in a single workspace:  screens, shared UI, and hooks are in `app/`, `components/`, `ui/`, `hooks/`, and supporting logic sits in `lib/` and `constants/`. PocketBase data, migrations, and hooks live in under the `server` directory in `pb_data/`, `pb_migrations/`, and `pb_hooks/`. End-to-end and unit tests reside in `tests/`.

## Code Style & Patterns

- Strive for simplicity and clarity
- Keep JSX minimal: No complex ternary operators, map functions, or calculations inside the return statement.
- Move logic out: All state management, event handling, and data processing must be in custom hooks (useFeatureName) or helper functions outside the main component function.
- Co-locate, don't embed: If logic is used only in the component, define it just above the JSX, keep the JSX clean of declarations and other logic
- Extract: If a sub-section of a function or JSX is complex, break it into separate, smaller parts.
- Conditional visibility: Instead of hiding/showing large blocks using `{condition && <Component />}`, have the component accept an `isVisible` prop and return null when it shouldn't render.
- Comments: Only add comments that explain "why", not "what". Avoid trivial comments like `// Delete users` before `deleteFrom('user')` or `// Create profile` before `insertInto('profile')`. If the code is self-explanatory, no comment is needed.
- Testing: Write unit tests for new features. Only mock using helpers in tests/unit.helpers.tsx as needed, do not mock out any of our own components or actions.
- Run quality checks after any changes:
   - `npm run checks` (Biome lint + format check + typecheck)
   - `npm run test:unit`
- Embrace Type Inference: Do not over-specify types, allow TypeScript to infer types whenever possible.
  - DO NOT USE `any` to pass type checks, even with biome ignore comments.
- Biome enforces 4-space indentation, single quotes, ES5 trailing commas, and no superfluous semicolons.
- Components export PascalCase React components (`CustomerList.tsx`), hooks use camelCase with a `use` prefix, and utility modules use kebab-case file names.
- This app supports both light and dark modes.  Do not use raw hex color values instead use a value from theme such as theme.colors.primary
- Keep hooks pure and side-effect free; call them at the top level of React components.
- **Avoid `useState` and `useEffect`** — almost always there is a better primitive:
  - Form fields → `useForm` + zod (see **Forms** section)
  - Server/async data → `useLiveQuery`
  - Mutations → `useMutation` from `~/lib/mutations`
  - Derived values → use `.select()` on liveQuery expressions to return computed values
  - Responding to prop/state changes → compute during render (not `useEffect` + `setState`)
  - DOM refs / imperative handles → `useRef`
  - Only reach for `useState` when you have genuinely local, synchronous UI state (e.g. a modal open/closed toggle, an accordion expanded state). If you find yourself pairing `useState` with `useEffect` to sync or transform data, that's a signal to use a better pattern.
- Use React Hook Form + Zod for forms — see the **Forms** section below for details.
- Captured exceptions should captured using `captureException` which can be imported from `~/lib/errors`
- Do not embed logic inside JSX. Prefer early return, assignment to variable that's inserted into JSX or other patterns to keep the code clean.
- Do not edit types/pbSchema.ts or types/pbZodSchema.ts, they are auto-generated whenever the database is migrated and edits will not be saved
- After developing a feature, offer to add it to the website's docs and feature sections 

## Data Queries & Mutations
- **ALWAYS use pbtsdb** for all PocketBase data queries and mutations - never use PocketBase directly in components
- Import collections with `useStore('collection1', 'collection2')` from `pbtsdb` - it uses variadic arguments and returns a tuple array
  - Example: `const [tagsCollection] = useStore('tags')`
  - Example: `const [jobsCollection, addressesCollection] = useStore('jobs', 'addresses')`
- Use `useLiveQuery` from `@tanstack/react-db` for realtime queries with automatic subscriptions
- Use TanStack DB operators (`eq`, `and`, `or`, `gt`, `lt`, etc.) from `@tanstack/db` for type-safe filtering
- Query syntax: `.from()`, `.where()`, `.orderBy()`, `.join()`, `.select()` - follows TanStack DB patterns
- **Prefer inline queries** — write `useStore` + `useLiveQuery` directly in the screen component rather than wrapping them in custom hooks. This keeps the data flow visible where it's used. Only extract a shared hook when the exact same query is needed in 3+ screens.
- **Mutations** — use `useMutation` from `~/lib/mutations` (not directly from `@tanstack/react-query`). It supports generator-based mutation functions that automatically await pbtsdb `Transaction` objects:
  ```ts
  const create = useMutation({
      mutationFn: function* (data) {
          yield contactsCollection.insert({ id: newRecordId(), ...data })
      },
      onSuccess: () => router.back(),
      onError: handleMutationErrorsWithForm({ setError, getValues }),
  })
  ```
- For multi-step mutations, yield each Transaction sequentially, or yield an array for parallel execution
- Use `performMutations` from `~/lib/mutations` when you need to await Transactions inside an async function
- All collections are configured in `lib/pocketbase.ts` with expand relations
- Reference documentation:
   - OneStack: https://onestack.dev/docs/introduction
   - pbtsdb: https://github.com/nathanstitt/pbtsdb/blob/main/llms.txt
   - TanStack DB: https://tanstack.com/db/latest/docs/overview

## Logging
- Use the centralized logger from `lib/logger.ts` instead of console.log
- Import with: `import { log } from '@/lib/logger'`
- Available log levels: `log.debug()`, `log.info()`, `log.warn()`, `log.error()`
- `log.error()` automatically sends errors to Sentry for tracking
- Use `log.debug()` for verbose development info, `log.info()` for general information, `log.warn()` for warnings, and `log.error()` for errors
- The logger shows timestamps and colors in development mode

## Scripts Reference
- `npm run dev` boots Expo (`expo start -p 7100`) and the PocketBase binary (`./jobtimey --dev`) together.
- `npm run expo:dev` starts Expo only; pair with remote PocketBase when debugging API calls.
- `npm run pb:dev` runs the PocketBase backend standalone.
- `npm run typecheck` runs `tsc --noEmit --skipLibCheck`.
- `npm run checks` runs lint and typechecks
- `npm run lint` (or `npm run lint:fix`) runs Biome linting/formatting.
- `npm run test:unit`, `npm run test:e2e`, and `npm run test:server` cover the Vitest suite, Playwright suite, and supporting services.
- `npm run test:e2e <test file>` will run a single test.  This will also start the dev server for testing. 

## PocketBase Notes
- Local data lives in `server/pb_data/`; reset via `tests/pb-test-server` scripts when fixtures fall out of sync.
- Keep migrations in `server/pb_migrations/` and describe manual steps in the PR body.
- Create api routes only as a last resort and after discussion. Prefer to create records using standard useMutation with pbtsdb stores.  If needed we can use golang hooks to observe and modify records as they're created/modified
- Go server hooks (e.g. CardDAV in `packages/contacts/server/`) use SDK methods that bypass PocketBase API rules — they implement equivalent authorization manually. When changing API rules on a collection, check if a Go hook also accesses that collection and update its filters to match.

## Users & Organizations
- Users belong to orgs via the `user_org` junction table (many-to-many)
- Roles (`admin`, `clerical`, `workforce`) are per-org—a user can have different roles in different orgs
- Use `useOrgInfo()` from `~/lib/use-org-info` to get `{ orgSlug, orgId, org }` for the current org context
- `useOrgSlug()` from `~/lib/use-org-slug` returns just the org slug — on web it reads from `OrgSlugContext` (set by `[orgSlug]/_layout.tsx`), on native it reads from AsyncStorage
- `navigateToOrg(orgSlug)` from `~/lib/org-url` does a same-origin path navigation to `/a/<orgSlug>`
- Session helpers like `getRoleForOrg(session, orgSlug)` provide role lookups

## Routing & Navigation
- **Org context comes from the URL path**: `/a/<orgSlug>/<service>` — e.g. `/a/acme/contacts`, `/a/acme/mail`, `/a/acme/settings/profile`
- All org-scoped routes use the `/a/[orgSlug]/` prefix in file-system routing
- Use `useOrgHref()` from `~/lib/org-routes` for **type-safe** org-scoped navigation. Pass short paths (without the `/a/[orgSlug]` prefix) — misspellings are caught at compile time:
  ```tsx
  const orgHref = useOrgHref()
  router.push(orgHref('contacts/new'))
  router.push(orgHref('contacts/[id]', { id: contact.id }))
  router.push(orgHref('mail', { folder: 'sent' }))
  <Link href={orgHref('mail/[id]', { id: threadId })} />
  ```
- **Never** use `as OneRouter.Href` casts for org routes — always use `useOrgHref()`
- For dynamic addon navigation where the slug is a runtime value (e.g. rail/tab bar), use the resolved URL string: `` `/a/${orgSlug}/${addonSlug}` ``
- Use `useOrgInfo()` or `useOrgSlug()` to get the current org — `useOrgSlug()` reads from context on web

## Add-on System
- Add-ons are npm packages (workspace or published) registered in `tinycld.addons.ts`
- `npm run addons:generate` (runs automatically before `dev` and `build:web`) wires addons into the app:
  - Re-exports addon screens into `app/a/[orgSlug]/{slug}/`
  - Generates typed collection wiring in `lib/generated/addon-collections.ts`
  - Generates addon registry in `lib/generated/addon-registry.ts`
  - Symlinks migrations/hooks into `server/pb_migrations/` and `server/pb_hooks/`
- Each addon provides: `manifest.ts`, `types.ts` (schema types), `collections.ts`, `screens/`, and optionally `pb-migrations/`, `pb-hooks/`, `seed.ts`, and `tests/`
- The type system is fully integrated — addon types.ts exports a `{PascalSlug}Schema` type that gets merged into `MergedSchema` so `useStore('addonCollection')` is strongly typed end-to-end
- Addon screens run in the app's bundle context and can import from the host app using `~/`
- `lib/generated/` and `app/a/[orgSlug]/*/` are gitignored; `app/a/[orgSlug]/_layout.tsx` and `app/a/[orgSlug]/settings/*` are core files (force-add to git)
- Runtime hooks: `useAddons()` and `useAddon(slug)` from `~/lib/addons/use-addons`
- Full documentation: `@docs/addons.md`

## Forms and other components
- All form UI components live in `ui/form/` and are exported from `~/ui/form`
- The barrel export re-exports `useForm`, `Control`, `Controller`, `zodResolver`, and `z` so screens only need one import:
  ```tsx
  import { useForm, zodResolver, z, TextInput, FormErrorSummary } from '~/ui/form'
  ```
- Available components: `TextInput`, `TextAreaInput`, `NumberInput`, `SelectInput`, `Toggle`, `FormErrorSummary`
- Each input accepts a generic `control` and type-safe `name` via `Path<T>` — pass the `control` from `useForm()` and field names are autocompleted
- Define a Zod schema per form, pass it via `zodResolver(schema)` to `useForm()`, and let TypeScript infer the form type from `defaultValues` — do not manually specify the generic
- Use `mode: 'onChange'` for real-time validation as the user types
- Show `<FormErrorSummary errors={errors} isEnabled={isSubmitted} />` above fields to surface all errors after first submit
- **Always use `useMutation` from `~/lib/mutations`** for form submissions — this ensures pbtsdb errors bubble up to the form via `onError`:
  ```ts
  const create = useMutation({
      mutationFn: function* (data) {
          yield collection.insert({ id: newRecordId(), ...data })
      },
      onSuccess: () => router.back(),
      onError: handleMutationErrorsWithForm({ setError, getValues }),
  })
  const onSubmit = handleSubmit((data) => create.mutate(data))
  ```
- Use `create.isPending` to disable the submit button and show loading state
- Error utilities in `lib/errors.ts`: `errorToString()`, `extractValidationErrors()`, `handleMutationErrorsWithForm()`, `captureException()`
- Do NOT use manual `useState` for form fields — always use `useForm` + the form components
- For complex forms, extract a `useFeatureForm()` hook that wraps `useForm` with schema, defaults, and submit logic
- See `packages/contacts/screens/new.tsx` for a reference implementation
- When developing a feature for a add-on, consider if the components you are adding would be of use to other add-ons. If so add them to the top-level ./components and offer to update other add-ons to use them.
- Always prefer to use tamagui components and styling.  full docs for tamagui are in @docs/tamagui.md

## Documentation & Support
- Expo documentation: https://docs.expo.dev/llms-full.txt
- PocketBase reference: https://raw.githubusercontent.com/Suryapratap-R/pocketbase-llm-txt/refs/heads/main/llms-full.txt
- PocketBase TS helper docs: https://raw.githubusercontent.com/satohshi/pocketbase-ts/refs/heads/master/README.md

## Project Structure & Module Organization
Expo routes live under `app/`, with organization screens in `app/a/[orgSlug]/` and shared layouts in `_layout.tsx`. Shared UI lives in `components/` and `ui/`, hooks in `hooks/`, and domain utilities in `lib/` and `constants/`. Static assets stay in `assets/` and `public/`. PocketBase data, migrations, and hooks live in `pb_data/`, `pb_migrations/`, and `pb_hooks/`. Tests and automation land in `tests/`, covering Playwright, Vitest, and Docker helpers.

