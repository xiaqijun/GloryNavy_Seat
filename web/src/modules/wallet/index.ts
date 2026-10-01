import { msg } from "@/lib/i18n";
import { Wallet } from "lucide-react";
import type { FrontendModule } from "@/app/module-registry";
export const walletModule: FrontendModule = {
  id: "wallet",
  apiVersion: 1,
  pages: [
    {
      id: "wallet.home",
      navigationGroup: "finance",
      path: "/wallet",
      label: msg("钱包"),
      icon: Wallet,
      load: () => import("./page"),
    },
  ],
};
