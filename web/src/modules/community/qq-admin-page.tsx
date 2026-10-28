import { House } from "lucide-react";
import { Link } from "react-router-dom";
import { msg } from "@/lib/i18n";
import { IconAction } from "@/components/ui/icon-action";
import { useSession } from "@/modules/identity";
import { QQBotSettings, QQGroupSettings } from "./qq-group-settings";

export default function QQAdminPage() {
  const session = useSession();
  const current = session.data?.session;

  if (!current) return null;
  return (
    <div className="community-admin-page">
      <div className="page-heading">
        <h1>{msg("QQ 机器人")}</h1>
        <div className="account-actions">
          <IconAction label={msg("返回工作台")} asChild>
            <Link to="/workspace">
              <House aria-hidden="true" />
            </Link>
          </IconAction>
        </div>
      </div>
      <div className="community-admin-grid">
        <QQBotSettings userID={current.user_id} csrf={current.csrf_token} />
        <QQGroupSettings userID={current.user_id} csrf={current.csrf_token} />
      </div>
    </div>
  );
}
