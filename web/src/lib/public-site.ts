import { msg } from "./i18n";

export const publicSiteURL = "https://seat.glorynavy.com/";
export const publicSiteLogo = `${publicSiteURL}images/glory-navy-logo.png`;
export const publicSiteImage = `${publicSiteURL}images/home-avatar.png`;

export function publicSiteMetadata() {
  const title = msg("荣耀海军 Glory Navy｜EVE Online 军团招募与成员服务");
  const description = msg(
    "荣耀海军（Glory Navy）是 EVE Online 宁静服玩家军团。了解 PVP 舰队、PVE 教学、工业生产、新人帮助、成员福利与入团要求，加入 QQ 招募群 674743797。",
  );
  return { title, description };
}

// Only stable facts already visible on the public homepage belong here.
// Current member counts, PAP, wallet data and benefits amounts are not embedded.
export function publicSiteStructuredData(language: string) {
  const { title, description } = publicSiteMetadata();
  const organization = `${publicSiteURL}#organization`;
  const website = `${publicSiteURL}#website`;
  return {
    "@context": "https://schema.org",
    "@graph": [
      {
        "@type": "Organization",
        "@id": organization,
        name: "Glory Navy",
        alternateName: ["荣耀海军", "GloryNavy", "GNV"],
        url: publicSiteURL,
        logo: publicSiteLogo,
        description,
        sameAs: ["https://zkillboard.com/corporation/98530802/"],
      },
      {
        "@type": "WebSite",
        "@id": website,
        url: publicSiteURL,
        name: "荣耀海军 Glory Navy",
        publisher: { "@id": organization },
        inLanguage: language,
      },
      {
        "@type": "WebPage",
        "@id": `${publicSiteURL}#webpage`,
        url: publicSiteURL,
        name: title,
        description,
        isPartOf: { "@id": website },
        about: { "@id": organization },
        inLanguage: language,
      },
    ],
  };
}

export type PageHeadItem = {
  tag: "meta" | "link" | "script";
  attrs: Record<string, string>;
  text?: string;
};

export function pageHeadItems(
  isPublic: boolean,
  language: string,
): PageHeadItem[] {
  const items: PageHeadItem[] = [
    {
      tag: "meta",
      attrs: {
        name: "robots",
        content: isPublic
          ? "index, follow, max-image-preview:large"
          : "noindex, nofollow",
      },
    },
  ];
  if (!isPublic) return items;
  const { title, description } = publicSiteMetadata();
  items.push(
    { tag: "meta", attrs: { name: "description", content: description } },
    { tag: "link", attrs: { rel: "canonical", href: publicSiteURL } },
    { tag: "meta", attrs: { property: "og:type", content: "website" } },
    {
      tag: "meta",
      attrs: { property: "og:site_name", content: "荣耀海军 Glory Navy" },
    },
    {
      tag: "meta",
      attrs: {
        property: "og:locale",
        content: language === "en" ? "en_US" : "zh_CN",
      },
    },
    { tag: "meta", attrs: { property: "og:url", content: publicSiteURL } },
    { tag: "meta", attrs: { property: "og:title", content: title } },
    {
      tag: "meta",
      attrs: { property: "og:description", content: description },
    },
    { tag: "meta", attrs: { property: "og:image", content: publicSiteImage } },
    { tag: "meta", attrs: { property: "og:image:type", content: "image/png" } },
    { tag: "meta", attrs: { property: "og:image:width", content: "1400" } },
    { tag: "meta", attrs: { property: "og:image:height", content: "980" } },
    {
      tag: "meta",
      attrs: {
        property: "og:image:alt",
        content: msg("EVE Online 神使级泰坦"),
      },
    },
    {
      tag: "meta",
      attrs: { name: "twitter:card", content: "summary_large_image" },
    },
    { tag: "meta", attrs: { name: "twitter:title", content: title } },
    {
      tag: "meta",
      attrs: { name: "twitter:description", content: description },
    },
    { tag: "meta", attrs: { name: "twitter:image", content: publicSiteImage } },
    {
      tag: "meta",
      attrs: {
        name: "twitter:image:alt",
        content: msg("EVE Online 神使级泰坦"),
      },
    },
    {
      tag: "script",
      attrs: { type: "application/ld+json" },
      text: JSON.stringify(publicSiteStructuredData(language)).replace(
        /</g,
        "\\u003c",
      ),
    },
  );
  return items;
}
