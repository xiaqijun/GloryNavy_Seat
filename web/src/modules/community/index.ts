import { Bot } from "lucide-react";
import { msg } from "@/lib/i18n";
import type { FrontendModule } from "@/app/module-registry";
export const communityModule: FrontendModule = {
  id: "community",
  apiVersion: 1,
  pages: [
    {
      id: "community.qq-admin",
      administratorOnly: true,
      navigationGroup: "administration",
      path: "/community",
      label: msg("QQ 机器人"),
      icon: Bot,
      load: () => import("./qq-admin-page"),
    },
  ],
};
