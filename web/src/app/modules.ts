import { skillsModule } from "@/modules/skills";
import { approvalModule } from "@/modules/approval";
import { walletModule } from "@/modules/wallet";
import { marketModule } from "@/modules/market";
import { welfareModule } from "@/modules/welfare";
import { exchangeModule } from "@/modules/exchange";
import { fittingsModule } from "@/modules/fittings";
import { attendanceModule } from "@/modules/attendance";
import { systemModule } from "@/modules/system";
import { eveModule } from "@/modules/eve";
import { communityModule } from "@/modules/community";
import { accessModule } from "@/modules/access";
import { sentryModule } from "@/modules/sentry";
import { registerModules } from "./module-registry";

// Explicit, reviewed imports. Adding a server module does not execute remote UI code.
export const frontendModules = registerModules([
  systemModule,
  approvalModule,
  attendanceModule,
  exchangeModule,
  fittingsModule,
  skillsModule,
  welfareModule,
  walletModule,
  marketModule,
  eveModule,
  communityModule,
  accessModule,
  sentryModule,
]);
