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
    <section
      id="strength"
      className="landing-strength"
      aria-labelledby="strength-title"
    >
      <div className="landing-width">
        <div className="landing-strength-heading landing-reveal">
          <h2 id="strength-title">{msg("军团实力")}</h2>
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
          {query.isError && (
            <span role="status">{msg("公开资料暂不可用")}</span>
          )}
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
    </section>
  );
}

export function FleetChapters() {
  return (
    <section
      id="fleet"
      className="landing-activities"
      aria-labelledby="fleet-title"
    >
      <div className="landing-width">
        <h2 id="fleet-title">{msg("军团活动与成员支持")}</h2>
        <div className="landing-activity-grid">
          {[
            {
              icon: Swords,
              title: "PVP 舰队",
              text: "收割、堵门、反收割与旗舰作战，由 FC 组织，和经验丰富的老玩家一起实战。",
            },
            {
              icon: Compass,
              title: "PVE 教学",
              text: "古斯塔斯异常与死亡空间教学舰队，帮助新人熟悉战斗与生存。",
            },
            {
              icon: Pickaxe,
              title: "工业生产",
              text: "月矿、冰矿与异常矿舰队，和军团生产团队一起协作。",
            },
            {
              icon: GraduationCap,
              title: "新人帮助",
              text: "保留认真对待每一位新人的传统，提供群内答疑与一对一指导。",
            },
            {
              icon: ShieldCheck,
              title: "补损与福利",
              text: "舰船补损、成长福利与旗舰补贴，申请条件和额度以站内当前规则为准。",
            },
            {
              icon: Gift,
              title: "PAP 与奖励",
              text: "活动积分按军团规则结算成果壳币，用于兑换物资奖励。",
            },
          ].map(({ icon: Icon, title, text }) => (
            <article className="landing-activity landing-reveal" key={title}>
              <Icon size={22} aria-hidden="true" />
              <h3>{msg(title)}</h3>
              <p>{msg(text)}</p>
            </article>
          ))}
        </div>
      </div>
    </section>
  );
}

export function JoiningSteps() {
  return (
    <section
      className="landing-steps landing-width"
      aria-labelledby="steps-title"
    >
      <h2 id="steps-title" className="sr-only">
        {msg("你的下一步")}
      </h2>
      <ol>
        {[
          [
            "01",
            "认识我们",
            "加入 QQ 或 KOOK 招募群，与招募人员聊聊你的游戏经历。",
          ],
          [
            "02",
            "加入军团",
            "了解入团安排后，在游戏中搜索 Glory Navy 并申请加入。",
          ],
          [
            "03",
            "准备出发",
            "通过 EVE 登录系统，绑定角色，查看配装与技能方案。",
          ],
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
