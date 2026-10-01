import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { ArrowUpRight, RefreshCw } from "lucide-react";
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
    refetchInterval: query => {
      const expires = query.state.data?.online?.expires_at;
      return expires ? Math.min(30_000, Math.max(1_000, Date.parse(expires) - Date.now() + 50)) : 30_000;
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
    const timer = setTimeout(() => setClock(Date.now()), Math.min(delay + 10, 2147483647));
    return () => clearTimeout(timer);
  }, [online]);
  const now = Math.max(clock, activity.dataUpdatedAt);
  const onlineFresh = online && Date.parse(online.expires_at) > now && !activity.isError;
  const current = combat?.months.find(m => m.month === new Date(now).toISOString().slice(0, 7));
  const shortNumber = (value: number) => value.toLocaleString(getLocale(), { notation: "compact", maximumFractionDigits: 1 });
  const stats = [
    [
      data ? data.member_count.toLocaleString(getLocale()) : "—",
      msg("军团角色"),
    ],
    [
      onlineFresh && online.characters !== null ? online.characters.toLocaleString(getLocale()) : "—",
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
          <div>
            <p className="landing-kicker">02 / {msg("军团实力")}</p>
            <h2 id="strength-title">{msg("不止一艘舰船。")}</h2>
          </div>
        </div>
        <dl className="landing-stats" aria-busy={query.isPending || activity.isPending}>
          {stats.map(([value, label]) => (
            <div key={label} className="landing-stat landing-reveal">
              <dt>{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
        <div className="landing-data-footer">
          {query.isError && <span role="status">{msg("公开资料暂不可用")}</span>}
          {!activity.isPending && !combat && <span role="status">{msg("公开战绩暂不可用")}</span>}
          {(data?.stale || combat?.stale || (activity.isError && combat)) && <span role="status">{msg("最近可用快照")}</span>}
          {(activity.isError || (!activity.isPending && !combat)) && <button className="landing-data-retry" onClick={() => void activity.refetch()}><RefreshCw size={14} aria-hidden="true" />{msg("重试")}</button>}
          {query.isError ? (
            <button
              className="landing-data-retry"
              onClick={() => void query.refetch()}
            >
              <RefreshCw size={14} aria-hidden="true" />
              {msg("重试")}
            </button>
          ) : null}
          <a
            href="https://zkillboard.com/corporation/98530802/"
            target="_blank"
            rel="noopener noreferrer"
          >
            {msg("查看公开战绩")}
            <ArrowUpRight size={15} aria-hidden="true" />
          </a>
        </div>
      </div>
    </section>
  );
}

export function FleetChapters() {
  return (
    <section
      id="fleet"
      className="landing-chapters"
      aria-labelledby="fleet-title"
    >
      <div className="landing-width landing-chapter-stage">
        <div className="landing-chapter-art" aria-hidden="true">
          <img
            src="/images/home-avatar.png"
            alt=""
            width="1400"
            height="980"
            loading="lazy"
          />
        </div>
        <div className="landing-chapter-content">
          <p className="landing-kicker">03 / {msg("舰队生活")}</p>
          <h2 id="fleet-title" className="sr-only">
            {msg("从集结，到下一次出发")}
          </h2>
          <div className="landing-chapter-panels">
            {[
              [
                "01",
                "集结。",
                "与舰队，一起出征。",
                "从确认配装到抵达集结点，在同一个频道里，找到属于你的作战席位。",
                "舰船配置",
                "活动出勤",
              ],
              [
                "02",
                "成长。",
                "每一次训练，都有方向。",
                "从技能要求到舰船方案，了解自己距离目标还差什么，把下一步变得清晰。",
                "技能方案",
                "达标检查",
              ],
              [
                "03",
                "再出发。",
                "让准备，跟上你的热爱。",
                "成员福利、补损与奖励兑换，让每一次申请都有记录，让下一次出发更有准备。",
                "军团福利",
                "奖励兑换",
              ],
            ].map(([number, title, sub, text, a, b]) => (
              <article className="landing-chapter-panel" key={number}>
                <span className="landing-chapter-number" aria-hidden="true">
                  {number}
                </span>
                <h3>{msg(title)}</h3>
                <h4>{msg(sub)}</h4>
                <p>{msg(text)}</p>
                <div className="landing-chapter-tags">
                  <span>{msg(a)}</span>
                  <span>{msg(b)}</span>
                </div>
              </article>
            ))}
          </div>
          <div className="landing-chapter-progress" aria-hidden="true">
            <span />
            <span />
            <span />
          </div>
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
      <div className="landing-reveal">
        <p className="landing-kicker">04 / {msg("你的下一步")}</p>
        <h2 id="steps-title">{msg("从认识彼此开始。")}</h2>
      </div>
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
