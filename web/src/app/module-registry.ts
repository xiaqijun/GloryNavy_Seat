import { lazy, type ComponentType, type LazyExoticComponent } from "react";
import type { LucideIcon } from "lucide-react";

export interface ModuleInfo {
  id: string;
  version: string;
  api_version: number;
}

export interface PageDefinition {
  id: string;
  path: string;
  label: string;
  icon: LucideIcon;
  navigation?: boolean;
  administratorOnly?: boolean;
  navigationGroup?: "operations" | "finance" | "benefits" | "administration";
  permission?:
    | "access.manage"
    | "eve.sync.manage"
    | "access.members.read"
    | "approval.self";
  layout?: "workspace" | "standalone";
  load: () => Promise<{ default: ComponentType }>;
}

export interface FrontendModule {
  id: string;
  apiVersion: number;
  required?: boolean;
  pages: PageDefinition[];
}

export interface RegisteredModule extends Omit<FrontendModule, "pages"> {
  pages: (Omit<PageDefinition, "load"> & {
    component: LazyExoticComponent<ComponentType>;
    preload: PageDefinition["load"];
  })[];
}

// Validate once at build registration; importing this file never loads page chunks.
export function registerModules(
  definitions: FrontendModule[],
): RegisteredModule[] {
  const ids = new Set<string>();
  const paths = new Set<string>();
  const pageIds = new Set<string>();
  return definitions.map((definition) => {
    if (
      !/^[a-z][a-z0-9-]*$/.test(definition.id) ||
      ids.has(definition.id) ||
      definition.apiVersion !== 1
    ) {
      throw new Error(`Invalid frontend module: ${definition.id}`);
    }
    ids.add(definition.id);
    return {
      ...definition,
      pages: definition.pages.map(({ load, ...page }) => {
        if (
          !page.id.startsWith(`${definition.id}.`) ||
          pageIds.has(page.id) ||
          paths.has(page.path) ||
          !/^\/(?:[a-z0-9-]+(?:\/[a-z0-9-]+)*)?$/.test(page.path)
        ) {
          throw new Error(`Invalid or duplicate page: ${page.id}`);
        }
        paths.add(page.path);
        pageIds.add(page.id);
        let pending: ReturnType<PageDefinition["load"]> | undefined;
        const preload = () => {
          pending ??= load().catch((error: unknown) => {
            pending = undefined;
            throw error;
          });
          return pending;
        };
        return { ...page, component: lazy(preload), preload };
      }),
    };
  });
}

// The server chooses active modules; navigation is not an authorization boundary.
export function selectPages(modules: RegisteredModule[], active: ModuleInfo[]) {
  const catalog = new Map(active.map((module) => [module.id, module]));
  return modules.flatMap((module) => {
    const backend = catalog.get(module.id);
    if (!backend) {
      if (module.required)
        throw new Error(`Required module unavailable: ${module.id}`);
      return [];
    }
    if (backend.api_version !== module.apiVersion)
      throw new Error(`Incompatible module: ${module.id}`);
    return module.pages;
  });
}
