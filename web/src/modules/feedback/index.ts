import { MessageSquarePlus } from "lucide-react";
import { msg } from "@/lib/i18n";
import type { FrontendModule } from "@/app/module-registry";
export const feedbackModule: FrontendModule = {
  id: "feedback",
  apiVersion: 1,
  pages: [
    {
      id: "feedback.requests",
      path: "/feedback",
      label: msg("意见反馈"),
      icon: MessageSquarePlus,
      load: () => import("./page"),
    },
  ],
};
