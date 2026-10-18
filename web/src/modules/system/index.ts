import { msg } from "@/lib/i18n";
import { Activity, ChartNoAxesCombined, LayoutDashboard } from "lucide-react";
import type { FrontendModule } from "@/app/module-registry";

export const systemModule: FrontendModule = {
  id: "system",
  apiVersion: 1,
  required: true,
  pages: [
    {
      id: "system.landing",
      path: "/",
      label: msg("GloryNavy 首页"),
      icon: LayoutDashboard,
      navigation: false,
      layout: "standalone",
      load: () => import("./landing-page"),
    },
    {
      id: "system.home",
      navigationOrder: 10,
      path: "/workspace",
      label: msg("工作台"),
      icon: LayoutDashboard,
      load: () => import("./home-page"),
    },
    {
      id: "system.operations",
      navigationOrder: 10,
      navigationGroup: "corporation",
      path: "/operations",
      label: msg("军团运营"),
      icon: ChartNoAxesCombined,
      permission: "access.manage",
      load: () => import("./operations-page"),
    },
    {
      id: "system.status",
      navigationOrder: 50,
      administratorOnly: true,
      navigationGroup: "administration",
      path: "/system",
      label: msg("系统状态"),
      icon: Activity,
      load: () => import("./status-page"),
    },
  ],
};
