import { getData } from "@/lib/http";
import type { ModuleInfo } from "./module-registry";

export function getModuleCatalog(signal?: AbortSignal) {
  return getData("/api/v1/modules", isCatalog, signal);
}

function isCatalog(value: unknown): value is ModuleInfo[] {
  if (!Array.isArray(value)) return false;
  const ids = new Set<string>();
  return value.every((item) => {
    if (
      !item ||
      typeof item.id !== "string" ||
      !/^[a-z][a-z0-9-]*$/.test(item.id) ||
      typeof item.version !== "string" ||
      !/^\d+\.\d+\.\d+$/.test(item.version) ||
      !Number.isInteger(item.api_version) ||
      item.api_version < 1 ||
      ids.has(item.id)
    )
      return false;
    ids.add(item.id);
    return true;
  });
}
