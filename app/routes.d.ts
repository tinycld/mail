// deno-lint-ignore-file
/* eslint-disable */
// biome-ignore: needed import
import type { OneRouter } from 'one'

declare module 'one' {
  export namespace OneRouter {
    export interface __routes<T extends string = string> extends Record<string, unknown> {
      StaticRoutes: 
        | `/`
        | `/_sitemap`
        | `/app`
        | `/app/contacts`
        | `/app/contacts/`
        | `/app/contacts/new`
        | `/app/mail`
        | `/app/mail/`
        | `/app/settings`
        | `/app/settings/`
        | `/app/settings/members`
        | `/app/settings/organization`
        | `/app/settings/profile`
        | `/tabs`
        | `/tabs/`
        | `/tabs/profile`
        | `/tabs/settings`
        | `/test`
      DynamicRoutes: 
        | `/app/contacts/${OneRouter.SingleRoutePart<T>}`
        | `/app/mail/${OneRouter.SingleRoutePart<T>}`
        | `/app/settings/${string}`
      DynamicRouteTemplate: 
        | `/app/contacts/[id]`
        | `/app/mail/[id]`
        | `/app/settings/[...section]`
      IsTyped: true
      RouteTypes: {
        '/app/contacts/[id]': RouteInfo<{ id: string }>
        '/app/mail/[id]': RouteInfo<{ id: string }>
        '/app/settings/[...section]': RouteInfo<{ section: string[] }>
      }
    }
  }
}

/**
 * Helper type for route information
 */
type RouteInfo<Params = Record<string, never>> = {
  Params: Params
  LoaderProps: { path: string; search?: string; subdomain?: string; params: Params; request?: Request }
}