import { Landmark } from "lucide-react";
import { msg } from "@/lib/i18n";
import type { FrontendModule } from "@/app/module-registry";

export const loanModule: FrontendModule = {
  id: "loan",
  apiVersion: 1,
  pages: [{ id: "loan.home", path: "/loans", label: msg("贷款"), icon: Landmark, navigationGroup: "finance", load: () => import("./page") }],
};
