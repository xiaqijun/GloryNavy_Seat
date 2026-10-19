import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import {
  ArrowUpRight,
  RefreshCw,
  Swords,
  Compass,
  Pickaxe,
  GraduationCap,
  ShieldCheck,
  Gift,
} from "lucide-react";
import { msg, getLocale } from "@/lib/i18n";
import { getPublicCorporation } from "./public-corporation-api";
import { getPublicActivity } from "./public-activity-api";

export function PublicStrength() {
  const query = useQuery({
    queryKey: ["public", "corporation"],
    queryFn: ({ signal }) => getPublicCorporation(signal),
    staleTime: 300_000,
    refetchInterval: 300_000,
    retry: 1,
  });
  const data = query.data;
  const activity = useQuery({
    queryKey: ["public", "corporation-activity"],
    queryFn: ({ signal }) => getPublicActivity(signal),
    staleTime: 15_000,
    refetchInterval: (query) => {
      const expires = query.state.data?.online?.expires_at;
      return expires
        ? Math.min(
            30_000,
            Math.max(1_000, Date.parse(expires) - Date.now() + 50),
          )
        : 30_000;
    },
    retry: 1,
  });
  const combat = activity.data?.combat;
  const online = activity.data?.online;
  const [clock, setClock] = useState(Date.now);
  useEffect(() => {
    if (!online) return;
    const delay = Date.parse(online.expires_at) - Date.now();
    if (delay <= 0) return;
    const timer = setTimeout(
      () => setClock(Date.now()),
      Math.min(delay + 10, 2147483647),
    );
    return () => clearTimeout(timer);
  }, [online]);
  const now = Math.max(clock, activity.dataUpdatedAt);
  const onlineFresh =
    online && Date.parse(online.expires_at) > now && !activity.isError;
  const current = combat?.months.find(
    (m) => m.month === new Date(now).toISOString().slice(0, 7),
  );
  const shortNumber = (value: number) =>
    value.toLocaleString("en-US", {
      notation: "compact",
      maximumFractionDigits: 1,
    });
  const stats = [
    [
      data ? data.member_count.toLocaleString(getLocale()) : "—",
      msg("军团角色"),
    ],
    [
      onlineFresh && online.characters !== null
        ? online.characters.toLocaleString(getLocale())
        : "—",
      msg("在线角色"),
    ],
    [current?.kills?.toLocaleString(getLocale()) ?? "—", msg("本月击毁")],
    [
      current?.value != null ? shortNumber(current.value) : "—",
      msg("击毁价值"),
    ],
  ];
  return (
    <div className="landing-strength" aria-labelledby="stats-title">
      <div className="landing-strength-heading">
        <h3 id="stats-title" className="sr-only">
          {msg("军团实力")}
        </h3>
        <a
          className="landing-text-link"
          href="https://zkillboard.com/corporation/98530802/"
          target="_blank"
          rel="noopener noreferrer"
        >
          {msg("查看公开战绩")}
          <ArrowUpRight size={15} aria-hidden="true" />
        </a>
      </div>
      <dl
        className="landing-stats"
        aria-busy={query.isPending || activity.isPending}
      >
        {stats.map(([value, label]) => (
          <div key={label} className="landing-stat landing-reveal">
            <dt>{label}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      <div className="landing-data-footer">
        {query.isError && <span role="status">{msg("公开资料暂不可用")}</span>}
        {!activity.isPending && !combat && (
          <span role="status">{msg("公开战绩暂不可用")}</span>
        )}
        {(data?.stale || combat?.stale || (activity.isError && combat)) && (
          <span role="status">{msg("最近可用快照")}</span>
        )}
        {(activity.isError || (!activity.isPending && !combat)) && (
          <button
            className="landing-data-retry"
            onClick={() => void activity.refetch()}
          >
            <RefreshCw size={14} aria-hidden="true" />
            {msg("重试")}
          </button>
        )}
        {query.isError ? (
          <button
            className="landing-data-retry"
            onClick={() => void query.refetch()}
          >
            <RefreshCw size={14} aria-hidden="true" />
            {msg("重试")}
          </button>
        ) : null}
      </div>
    </div>
  );
}

export function FleetChapters({
  variant = "fleet",
}: {
  variant?: "fleet" | "support";
}) {
  const support = variant === "support";
  return (
    <section
      id={variant}
      className={`landing-page landing-activities${support ? " landing-support" : ""}`}
      aria-labelledby={`${variant}-title`}
    >
      <div className="landing-width landing-page-content">
        <h2 id={`${variant}-title`}>
          {msg(support ? "成员保障" : "军团活动")}
        </h2>
        <div className="landing-activity-grid">
          {[
            {
              icon: Swords,
              title: "PVP 舰队",
              text: "收割、堵门、反收割与旗舰作战。",
            },
            {
              icon: Compass,
              title: "PVE 教学",
              text: "古斯塔斯异常与死亡空间教学。",
            },
            {
              icon: Pickaxe,
              title: "工业生产",
              text: "月矿、冰矿与异常矿舰队。",
            },
            {
              icon: GraduationCap,
              title: "新人帮助",
              text: "群内答疑，一对一指导。",
            },
            {
              icon: ShieldCheck,
              title: "补损与福利",
              text: "舰船补损、成长福利、旗舰补贴。",
            },
            {
              icon: Gift,
              title: "PAP 与奖励",
              text: "出勤积累 PAP，结算果壳币兑换物资。",
            },
          ]
            .slice(support ? 3 : 0, support ? 6 : 3)
            .map(({ icon: Icon, title, text }, index) => (
              <article
                className={`landing-activity${!support && index === 0 ? " landing-activity-featured" : ""}`}
                key={title}
              >
                <span className="landing-activity-icon">
                  <Icon size={28} aria-hidden="true" />
                </span>
                <div className="landing-activity-copy">
                  <h3>{msg(title)}</h3>
                  <p>{msg(text)}</p>
                </div>
              </article>
            ))}
        </div>
        {support && (
          <p className="landing-policy">
            {msg("福利条件与额度以站内当前规则为准。")}
          </p>
        )}
      </div>
    </section>
  );
}

export function JoiningSteps() {
  return (
    <section className="landing-steps" aria-labelledby="steps-title">
      <h2 id="steps-title" className="sr-only">
        {msg("你的下一步")}
      </h2>
      <ol>
        {[
          ["01", "认识我们", "联系招募群管理员。"],
          ["02", "加入军团", "游戏内申请 Glory Navy。"],
          ["03", "准备出发", "登录本站，绑定角色。"],
        ].map(([num, title, text]) => (
          <li key={num} className="landing-reveal">
            <span aria-hidden="true">{num}</span>
            <h3>{msg(title)}</h3>
            <p>{msg(text)}</p>
          </li>
        ))}
      </ol>
    </section>
  );
}
