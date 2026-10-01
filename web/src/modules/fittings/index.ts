import { msg } from "@/lib/i18n";
import { Orbit } from "lucide-react";
import type { FrontendModule } from "@/app/module-registry";
export const fittingsModule: FrontendModule = {
  id: "fittings",
  apiVersion: 1,
  pages: [
    {
      id: "fittings.overview",
      navigationGroup: "operations",
      path: "/fittings",
      label: msg("舰船配置"),
      icon: Orbit,
      load: () => import("./library-page"),
    },
  ],
};
