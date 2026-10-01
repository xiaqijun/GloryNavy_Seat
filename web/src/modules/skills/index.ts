import { msg } from "@/lib/i18n";
import { GraduationCap } from "lucide-react";
import type { FrontendModule } from "@/app/module-registry";
export const skillsModule: FrontendModule = {
  id: "skills",
  apiVersion: 1,
  pages: [
    {
      id: "skills.overview",
      navigationGroup: "operations",
      path: "/skills",
      label: msg("技能管理"),
      icon: GraduationCap,
      load: () => import("./page"),
    },
  ],
};
