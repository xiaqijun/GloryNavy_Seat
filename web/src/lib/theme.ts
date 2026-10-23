import { useSyncExternalStore } from "react";

export type ThemePreference = "system" | "light" | "dark";
export const themeStorageKey = "glorynavy.theme";
const listeners = new Set<() => void>();
let preference: ThemePreference = "system";
let resolved: "light" | "dark" = "light";
const valid = (value: unknown): value is ThemePreference =>
  value === "system" || value === "light" || value === "dark";

function readPreference(): ThemePreference {
  try {
    const value = localStorage.getItem(themeStorageKey);
    return valid(value) ? value : "system";
  } catch {
    return "system";
  }
}

function apply() {
  resolved = preference === "system"
    ? matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"
    : preference;
  document.documentElement.dataset.theme = resolved;
  document.documentElement.style.colorScheme = resolved;
  listeners.forEach((notify) => notify());
}

export function initializeTheme() {
  preference = readPreference();
  apply();
  const media = matchMedia("(prefers-color-scheme: dark)");
  const onSystem = () => { if (preference === "system") apply(); };
  const onStorage = (event: StorageEvent) => {
    if (event.storageArea === localStorage &&
      (event.key === themeStorageKey || event.key === null)) {
      preference = readPreference();
      apply();
    }
  };
  media.addEventListener("change", onSystem);
  window.addEventListener("storage", onStorage);
  return () => {
    media.removeEventListener("change", onSystem);
    window.removeEventListener("storage", onStorage);
  };
}

export function setThemePreference(next: string) {
  if (!valid(next)) return;
  preference = next;
  try { localStorage.setItem(themeStorageKey, next); } catch { /* Optional preference. */ }
  apply();
}

const subscribe = (listener: () => void) => {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
};
export const useThemePreference = () =>
  useSyncExternalStore(subscribe, () => preference, () => "system" as ThemePreference);
export const useResolvedTheme = () =>
  useSyncExternalStore(subscribe, () => resolved, () => "light" as const);
