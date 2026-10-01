import type { RegisteredModule } from "./module-registry";

type Page = RegisteredModule["pages"][number];

// Warm only authorized navigation code, one chunk per idle period. Business data
// remains page-owned and is never fetched for a route the member has not opened.
export function warmNavigationPages(
  pages: Page[],
  currentPath: string,
  isDataLoading: () => boolean = () => false,
) {
  const activeGroup = pages.find((page) => page.path === currentPath)?.navigationGroup;
  const queue = pages
    .filter((page) => page.navigation !== false && page.path !== currentPath)
    .sort((left, right) =>
      Number(right.navigationGroup === activeGroup) -
      Number(left.navigationGroup === activeGroup),
    );
  let stopped = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let idle: number | undefined;

  const next = () => {
    timer = undefined;
    if (stopped || queue.length === 0) return;
    if (document.visibilityState !== "visible" || isDataLoading()) {
      timer = setTimeout(next, 500);
      return;
    }
    const loadOne = () => {
      timer = undefined;
      idle = undefined;
      if (stopped) return;
      if (document.visibilityState !== "visible" || isDataLoading()) {
        timer = setTimeout(next, 500);
        return;
      }
      const page = queue.shift()!;
      void Promise.resolve()
        .then(() => page.preload())
        .catch(() => {})
        .finally(() => {
          if (!stopped) timer = setTimeout(next, 250);
        });
    };
    if (typeof window.requestIdleCallback === "function") {
      idle = window.requestIdleCallback(loadOne);
    } else {
      timer = setTimeout(loadOne, 0);
    }
  };

  if (queue.length > 0) timer = setTimeout(next, 1200);
  return () => {
    stopped = true;
    if (timer !== undefined) clearTimeout(timer);
    if (idle !== undefined) window.cancelIdleCallback(idle);
  };
}
