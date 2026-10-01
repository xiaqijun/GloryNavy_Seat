import { msg } from "@/lib/i18n";
import { useQuery } from "@tanstack/react-query";
import { LoaderCircle, RefreshCw, ArrowLeft } from "lucide-react";
import { Link, Navigate } from "react-router-dom";
import { Card, CardContent } from "@/components/ui/card";
import { IconAction } from "@/components/ui/icon-action";
import { useSession } from "@/modules/identity";
import { getLoginStatus } from "./api";
import { EveSignInButton } from "./login-components";
import { useLoginError } from "./use-login-error";

export default function LoginPage() {
  const auth = useSession();
  const status = useQuery({
    queryKey: ["eve", "login-status"],
    queryFn: ({ signal }) => getLoginStatus(signal),
  });
  const error = useLoginError();
  const loading = auth.isPending || status.isPending;
  const unavailable = auth.isError || status.isError;
  if (auth.isSuccess && auth.data.session && !error)
    return <Navigate to="/account" replace />;
  return (
    <div className="auth-page">
      <Card className="auth-card">
        <div className="auth-brand-art" aria-hidden="true">
          <img
            src="/images/login-cruiser.webp"
            alt=""
            width={1448}
            height={1086}
            fetchPriority="high"
            decoding="async"
          />
        </div>
        <div className="auth-panel">
          <Link
            to="/"
            className="brand auth-brand"
            aria-label={msg("GloryNavy 首页")}
          >
            <img
              className="auth-corporation-logo"
              src="/images/glory-navy-logo.png"
              alt=""
              width={48}
              height={48}
            />
            <span>GloryNavy</span>
          </Link>
          <CardContent className="auth-content">
            <h1 className="sr-only">{msg("EVE 登录")}</h1>
            {error && (
              <p role="alert" className="login-error">
                {error}
              </p>
            )}
            {loading ? (
              <span
                role="status"
                aria-label={msg("正在检查登录状态")}
                aria-busy="true"
                className="auth-loading"
              >
                <LoaderCircle className="animate-spin" aria-hidden="true" />
              </span>
            ) : unavailable ? (
              <div role="alert" className="auth-feedback">
                <p>{msg("登录服务暂不可用，请重试。")}</p>
                <IconAction
                  label={msg("重试登录状态")}
                  onClick={() => {
                    void auth.refetch();
                    void status.refetch();
                  }}
                >
                  <RefreshCw aria-hidden="true" />
                </IconAction>
              </div>
            ) : !status.data?.configured ? (
              <p role="status" className="muted">
                {msg("登录尚未配置，请联系管理员。")}{" "}
              </p>
            ) : (
              <EveSignInButton />
            )}
            {auth.data?.session && error && (
              <IconAction label={msg("返回我的角色")} asChild>
                <Link to="/account">
                  <ArrowLeft aria-hidden="true" />
                </Link>
              </IconAction>
            )}
          </CardContent>
        </div>
      </Card>
    </div>
  );
}
