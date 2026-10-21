import { Coins } from "lucide-react";
import { msg } from "@/lib/i18n";
import type { FrontendModule } from "@/app/module-registry";
export const lpModule: FrontendModule = {
  id: "lp", apiVersion: 1,
  pages: [{ id: "lp.query", path: "/lp", label: msg("LP 兑换查询"), icon: Coins,
    navigationGroup: "tools", navigationOrder: 10, load: () => import("./page") }],
};
