import { msg } from "@/lib/i18n";
import { RadioTower } from "lucide-react";
import type { FrontendModule } from "@/app/module-registry";

export const sentryModule: FrontendModule = {
  id: "sentry",
  apiVersion: 1,
  pages: [
    {
      id: "sentry.keys",
      path: "/sentry",
      label: msg("预警平台"),
      icon: RadioTower,
      navigationGroup: "operations",
      load: () => import("./page"),
    },
  ],
};
