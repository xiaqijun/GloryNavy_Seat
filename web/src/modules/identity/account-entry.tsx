import { msg } from "@/lib/i18n";
import { LogIn, UserRound } from "lucide-react";
import { Link } from "react-router-dom";
import { IconAction } from "@/components/ui/icon-action";
import { useSession } from "./use-session";

export function AccountEntry() {
  const query = useSession();
  const name =
    query.data?.session?.main_character?.name ??
    query.data?.session?.character.name;
  return (
    <IconAction label={name ? msg("账号：{0}", name) : msg("EVE 登录")} asChild>
      <Link to={name ? "/account" : "/"}>
        {name ? <UserRound aria-hidden="true" /> : <LogIn aria-hidden="true" />}
      </Link>
    </IconAction>
  );
}
