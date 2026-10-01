import { msg, getLocale } from "@/lib/i18n";
import { useQuery } from "@tanstack/react-query";
import {
  Database,
  RefreshCw,
  Hourglass,
  Server,
  Globe2,
  Users,
} from "lucide-react";
import { IconAction } from "@/components/ui/icon-action";
import { Card, CardContent } from "@/components/ui/card";
import { APIError } from "@/lib/http";
import { getSystemStatus } from "./api";

export default function Workspace({ system = false }: { system?: boolean }) {
  const query = useQuery({
    queryKey: ["system", "status"],
    queryFn: ({ signal }) => getSystemStatus(signal),
  });
  const ready = query.isSuccess && !query.isError;
  return (
    <>
      <div className="page-heading">
        <h1>{system ? msg("系统状态") : msg("军团工作台")}</h1>
        <IconAction
          label={query.isFetching ? msg("正在刷新") : msg("刷新状态")}
          disabled={query.isFetching}
          aria-busy={query.isFetching}
          onClick={() => void query.refetch()}
        >
          {query.isFetching ? (
            <Hourglass aria-hidden="true" />
          ) : (
            <RefreshCw aria-hidden="true" />
          )}
        </IconAction>
      </div>
      {query.isError && (
        <div role="alert" className="error-panel">
          <p>
            {query.error instanceof APIError
              ? query.error.message
              : msg("无法连接服务，请检查网络后重试。")}
          </p>
          {query.error instanceof APIError && query.error.requestId && (
            <p className="request-id">
              {msg("请求编号：")}
              {query.error.requestId}
            </p>
          )}
        </div>
      )}
      <section
        className="status-grid"
        aria-label={msg("平台概览")}
        aria-live="polite"
        aria-busy={query.isFetching}
      >
        {[
          {
            title: msg("平台服务"),
            icon: Server,
            value: query.isPending
              ? msg("正在检查")
              : ready
                ? msg("连接正常")
                : msg("连接异常"),
            state: ready ? "success" : query.isError ? "error" : "neutral",
          },
          {
            title: msg("数据库"),
            icon: Database,
            value: ready
              ? msg("已就绪")
              : query.isPending
                ? msg("正在检查")
                : msg("无法确认"),
            state: ready ? "success" : "neutral",
          },
          {
            title: msg("游戏数据"),
            icon: Globe2,
            value: msg("未接入"),
            state: "neutral",
          },
        ].map(({ title, icon: Icon, value, state }) => (
          <Card key={title} className="status-card">
            <CardContent>
              <div className="status-label">
                <span className="icon-box">
                  <Icon size={18} aria-hidden="true" />
                </span>
                <span>{title}</span>
              </div>
              <div className="status-value">
                <span className={`status-dot ${state}`} />
                {value}
              </div>
            </CardContent>
          </Card>
        ))}
      </section>
      {system ? (
        <Card className="system-details-card">
          <CardContent>
            <h2>{msg("运行信息")}</h2>
            <dl className="details">
              <div>
                <dt>{msg("服务版本")}</dt>
                <dd>{query.data?.version ?? msg("待获取")}</dd>
              </div>
              <div>
                <dt>{msg("数据结构版本")}</dt>
                <dd>
                  {query.data ? `v${query.data.schema_version}` : msg("待获取")}
                </dd>
              </div>
              <div>
                <dt>{msg("上次成功检查")}</dt>
                <dd>
                  {query.dataUpdatedAt ? (
                    <time
                      dateTime={new Date(query.dataUpdatedAt).toISOString()}
                    >
                      {new Date(query.dataUpdatedAt).toLocaleString(
                        getLocale(),
                        {
                          hour12: false,
                        },
                      )}
                    </time>
                  ) : (
                    msg("尚未检查")
                  )}
                </dd>
              </div>
            </dl>
          </CardContent>
        </Card>
      ) : (
        <Card className="workspace-empty-card">
          <CardContent className="empty-state">
            <span className="empty-icon">
              <Users size={24} aria-hidden="true" />
            </span>
            <h2>{msg("暂无军团数据")}</h2>
          </CardContent>
        </Card>
      )}
    </>
  );
}
