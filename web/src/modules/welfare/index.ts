import { msg } from "@/lib/i18n";
import { HeartHandshake, Crosshair } from "lucide-react";
import type { FrontendModule } from "@/app/module-registry";
export const welfareModule: FrontendModule = {
  id: "welfare",
  apiVersion: 1,
  pages: [
    {
      id: "welfare.losses",
      navigationGroup: "operations",
      path: "/losses",
      label: msg("舰船损失"),
      icon: Crosshair,
      load: () => import("./losses-page"),
    },
    {
      id: "welfare.home",
      navigationGroup: "benefits",
      path: "/welfare",
      label: msg("军团福利"),
      icon: HeartHandshake,
      load: () => import("./page"),
    },
  ],
};
