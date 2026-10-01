import { msg } from "@/lib/i18n";
import { UserRound, RefreshCw, FileText } from "lucide-react";
import type { FrontendModule } from "@/app/module-registry";
export const eveModule: FrontendModule = {
  id: "eve",
  apiVersion: 1,
  pages: [
    {
      id: "eve.contracts",
      navigationGroup: "finance",
      path: "/contracts",
      label: msg("合同"),
      icon: FileText,
      load: () => import("./contracts-page"),
    },
    {
      id: "eve.sync",
      navigationGroup: "administration",
      path: "/sync",
      label: msg("ESI 同步"),
      icon: RefreshCw,
      permission: "eve.sync.manage",
      load: () => import("./sync-page"),
    },
    {
      id: "eve.login",
      path: "/login",
      label: msg("EVE 登录"),
      icon: UserRound,
      navigation: false,
      layout: "standalone",
      load: () => import("./login-page"),
    },
    {
      id: "eve.account",
      path: "/account",
      label: msg("我的角色"),
      icon: UserRound,
      navigation: false,
      load: () => import("./account-page"),
    },
  ],
};
