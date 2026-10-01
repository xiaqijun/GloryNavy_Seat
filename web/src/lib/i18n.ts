import { english } from "./locales/en";

export type Locale = "zh-CN" | "en";
export const localeStorageKey = "glorynavy.locale";
const supported = (value: string | null): value is Locale =>
  value === "zh-CN" || value === "en";

export function readLocale(): Locale {
  if (typeof window === "undefined") return "zh-CN";
  const query = new URL(window.location.href).searchParams.get("lang");
  if (supported(query)) return query;
  try {
    const saved = window.localStorage.getItem(localeStorageKey);
    if (supported(saved)) return saved;
  } catch {
    /* Browser privacy settings can disable storage. */
  }
  return "zh-CN";
}

// One locale per document also covers module-level labels and workers consistently.
const locale = readLocale();
export const getLocale = (): Locale => locale;

export function translate(
  language: Locale,
  source: string,
  ...values: unknown[]
): string {
  const template =
    language === "en" && Object.hasOwn(english, source)
      ? english[source]
      : source;
  return template.replace(/\{(\d+)\}/g, (placeholder, index: string) =>
    Number(index) < values.length
      ? String(values[Number(index)] ?? "")
      : placeholder,
  );
}

export const msg = (source: string, ...values: unknown[]) =>
  translate(locale, source, ...values);

export function changeLocale(next: Locale): void {
  if (next === locale) return;
  const url = new URL(window.location.href);
  try {
    window.localStorage.setItem(localeStorageKey, next);
    url.searchParams.delete("lang");
  } catch {
    url.searchParams.set("lang", next);
  }
  // Replacing an unchanged URL with a fragment may only navigate within the
  // document. Commit the URL without adding history, then force a real reload.
  window.history.replaceState(window.history.state, "", url.href);
  window.location.reload();
}

export function initializeLocale(): void {
  document.documentElement.lang = locale;
  try {
    window.localStorage.setItem(localeStorageKey, locale);
  } catch {
    /* optional preference */
  }
}
