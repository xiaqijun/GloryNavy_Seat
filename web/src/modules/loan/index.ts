import { ClipboardList, Landmark } from "lucide-react";
import { msg } from "@/lib/i18n";
import type { FrontendModule } from "@/app/module-registry";

export const loanModule: FrontendModule = {
  id: "loan",
  apiVersion: 1,
  pages: [
    {
      id: "loan.home",
      path: "/loans",
      label: msg("贷款看板"),
      icon: Landmark,
      navigation: false,
      load: () => import("./dashboard-page"),
    },
    {
      id: "loan.operations",
      navigationOrder: 40,
      path: "/loans/operations",
      label: msg("贷款"),
      icon: ClipboardList,
      navigationGroup: "finance",
      load: () => import("./page"),
    },
  ],
};
