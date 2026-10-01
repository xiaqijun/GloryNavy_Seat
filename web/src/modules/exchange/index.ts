import { msg } from "@/lib/i18n";
import { Coins, Library } from "lucide-react";
import type { FrontendModule } from "@/app/module-registry";
export const exchangeModule: FrontendModule = {
  id: "exchange",
  apiVersion: 1,
  pages: [
    {
      id: "exchange.catalog",
      administratorOnly: true,
      navigationGroup: "benefits",
      path: "/rewards",
      label: msg("奖励库"),
      icon: Library,
      load: () => import("./catalog-page"),
    },
    {
      id: "exchange.overview",
      navigationGroup: "benefits",
      path: "/exchange",
      label: msg("兑换中心"),
      icon: Coins,
      load: () => import("./page"),
    },
  ],
};
