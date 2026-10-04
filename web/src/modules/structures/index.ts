import { msg } from "@/lib/i18n";
import { Building2 } from "lucide-react";
import type { FrontendModule } from "@/app/module-registry";

export const structuresModule: FrontendModule = {
  id: "structures",
  apiVersion: 1,
  pages: [
    {
      id: "structures.overview",
      path: "/structures",
      label: msg("建筑管理"),
      icon: Building2,
      navigationGroup: "operations",
      load: () => import("./page"),
    },
  ],
};
