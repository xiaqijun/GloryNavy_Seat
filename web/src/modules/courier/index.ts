import { Truck } from "lucide-react";
import { msg } from "@/lib/i18n";
import type { FrontendModule } from "@/app/module-registry";
export const courierModule: FrontendModule = {
  id: "courier",
  apiVersion: 1,
  pages: [
    {
      id: "courier.service",
      navigationOrder: 50,
      path: "/courier",
      label: msg("快递服务"),
      icon: Truck,
      navigationGroup: "finance",
      load: () => import("./page"),
    },
  ],
};
