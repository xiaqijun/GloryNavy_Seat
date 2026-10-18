import { Bell } from "lucide-react";
import { msg } from "@/lib/i18n";
import type { FrontendModule } from "@/app/module-registry";
export const reminderModule: FrontendModule = {
  id: "reminder",
  apiVersion: 1,
  pages: [
    {
      id: "reminder.manage",
      navigationOrder: 20,
      administratorOnly: true,
      navigationGroup: "administration",
      path: "/reminders",
      label: msg("提醒"),
      icon: Bell,
      load: () => import("./page"),
    },
  ],
};
