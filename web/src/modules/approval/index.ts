import { ClipboardCheck } from "lucide-react";
import { msg } from "@/lib/i18n";
import type { FrontendModule } from "@/app/module-registry";
export const approvalModule: FrontendModule = {
  id: "approval",
  apiVersion: 1,
  pages: [
    {
      id: "approval.queue",
      path: "/approvals",
      label: msg("审批中心"),
      icon: ClipboardCheck,
      permission: "approval.self",
      load: () => import("./page"),
    },
  ],
};
