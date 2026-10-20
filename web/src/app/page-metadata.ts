import { useEffect } from "react";
import { getLocale, msg } from "@/lib/i18n";
import { pageHeadItems, publicSiteMetadata } from "@/lib/public-site";

export function usePageMetadata(pathname: string) {
  useEffect(() => {
    const isPublic = pathname === "/";
    // Build output and client navigation use the same managed tags.
    document.head
      .querySelectorAll("[data-site-meta]")
      .forEach((node) => node.remove());
    for (const item of pageHeadItems(isPublic, getLocale())) {
      const node = document.createElement(item.tag);
      node.setAttribute("data-site-meta", "");
      for (const [key, value] of Object.entries(item.attrs))
        node.setAttribute(key, value);
      if (item.text) node.textContent = item.text;
      document.head.append(node);
    }
    if (isPublic) document.title = publicSiteMetadata().title;
    else if (document.title === publicSiteMetadata().title)
      document.title = msg("GloryNavy · 军团管理平台");
  }, [pathname]);
}
