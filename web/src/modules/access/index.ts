import { msg } from "@/lib/i18n";
export { AuthorizationSummary } from "./authorization-summary";
import { ShieldCheck, UsersRound } from "lucide-react";
import type { FrontendModule } from "@/app/module-registry";
export { useManagementAccess } from "./use-management-access";
export const accessModule: FrontendModule = {
  id: "access",
  apiVersion: 1,
  pages: [
    {
      id: "access.members",
      navigationGroup: "administration",
      path: "/members",
      label: msg("成员"),
      icon: UsersRound,
      permission: "access.members.read",
      load: () => import("./members-page"),
    },
    {
      id: "access.management",
      navigationGroup: "administration",
      path: "/access",
      label: msg("权限管理"),
      icon: ShieldCheck,
      permission: "access.manage",
      load: () => import("./management-page"),
    },
  ],
};
