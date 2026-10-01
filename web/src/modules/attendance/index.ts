import { msg } from "@/lib/i18n";
import { CalendarCheck2 } from "lucide-react";
import type { FrontendModule } from "@/app/module-registry";
export const attendanceModule: FrontendModule = {
  id: "attendance",
  apiVersion: 1,
  pages: [
    {
      id: "attendance.overview",
      navigationGroup: "operations",
      path: "/attendance",
      label: msg("军团考勤"),
      icon: CalendarCheck2,
      load: () => import("./page"),
    },
  ],
};
