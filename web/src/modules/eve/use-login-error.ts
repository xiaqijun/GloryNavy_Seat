import { msg } from "@/lib/i18n";
import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";

const errors: Record<string, string> = {
  merge_rejected: msg(
    "无法合并该账号，请选择其他已绑定本站且身份正常的账号重新验证。",
  ),
  cancelled: msg("已取消登录，可以重新尝试。"),
  invalid_state: msg("登录请求已过期或失效，请重新登录。"),
  unavailable: msg("EVE 登录暂不可用，请稍后重试。"),
  not_configured: msg("登录尚未配置，请联系管理员。"),
  identity_rejected: msg("角色身份暂无法确认，请重试或联系管理员。"),
  authorization_required: msg("授权范围不完整，请确认应用权限后重新授权。"),
  character_conflict: msg(
    "该角色已属于其他本站账号，可使用“合并账号”验证后迁入。",
  ),
  session_expired: msg("原登录已失效，请重新登录后添加角色。"),
  wrong_character: msg("选择的角色不符，请选择需要更新授权的角色。"),
};
export function useLoginError(account = false) {
  const [params, setParams] = useSearchParams();
  const [error] = useState(() =>
    account && params.get("error") === "cancelled"
      ? msg("已取消授权，角色绑定未改变。")
      : errors[params.get("error") ?? ""],
  );
  useEffect(() => {
    if (params.has("error")) setParams({}, { replace: true });
  }, [params, setParams]);
  return error;
}
