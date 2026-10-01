import { msg } from "@/lib/i18n";
import { Calculator } from "lucide-react";
import type { FrontendModule } from "@/app/module-registry";
export const marketModule: FrontendModule = {
  id: "market",
  apiVersion: 1,
  pages: [
    {
      id: "market.appraisal",
      path: "/appraisal",
      label: msg("物品估价"),
      icon: Calculator,
      navigationGroup: "finance",
      load: () => import("./page"),
    },
  ],
};
