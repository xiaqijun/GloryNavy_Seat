import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { ArrowDown, ArrowUp, ArrowUpRight, Check, Copy } from "lucide-react";
import { LanguageSwitch } from "@/components/language-switch";
import { useSession } from "@/modules/identity";
import { EveSignInButton } from "@/modules/eve/login-components";
import { useLoginError } from "@/modules/eve/use-login-error";
import { msg } from "@/lib/i18n";
import {
  PublicStrength,
  FleetChapters,
  JoiningSteps,
} from "./landing-sections";
import "./landing.css";

const landingSections = [
  { id: "home", label: "首页" },
  { id: "strength", label: "军团介绍" },
  { id: "fleet", label: "军团活动" },
  { id: "support", label: "成员保障" },
  { id: "join", label: "加入我们" },
] as const;

export default function LandingPage() {
  const root = useRef<HTMLDivElement>(null);
  const [activeSection, setActiveSection] = useState("home");
  const session = useSession();
  const loginError = useLoginError();
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">(
    "idle",
  );
  const activeIndex = landingSections.findIndex(
    ({ id }) => id === activeSection,
  );

  useEffect(() => {
    const element = root.current;
    if (!element) return;
    const sections = Array.from(
      element.querySelectorAll<HTMLElement>(".landing-page"),
    );
    const header = element.querySelector<HTMLElement>(".landing-topbar")!;
    const html = document.documentElement;
    const previousHeight = html.style.getPropertyValue(
      "--landing-header-height",
    );
    let frame = 0;
    let layoutFrame = 0;
    let headerHeight = 0;
    let positions: { id: string; top: number }[] = [];
    let active = "";
    const update = () => {
      frame = 0;
      const boundary =
        window.scrollY +
        headerHeight +
        (window.innerHeight - headerHeight) * 0.4;
      let current = "home";
      for (const position of positions) {
        if (position.top <= boundary) current = position.id;
      }
      if (current === active) return;
      active = current;
      for (const section of sections)
        section.dataset.active = String(section.id === current);
      setActiveSection(current);
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(update);
    };
    // Measure only when layout changes, not on every scroll frame.
    const measure = () => {
      layoutFrame = 0;
      const nextHeight = header.getBoundingClientRect().height;
      if (nextHeight !== headerHeight) {
        headerHeight = nextHeight;
        html.style.setProperty("--landing-header-height", `${headerHeight}px`);
      }
      positions = sections.map((section) => ({
        id: section.id,
        top: section.getBoundingClientRect().top + window.scrollY,
      }));
      schedule();
    };
    const scheduleMeasure = () => {
      if (!layoutFrame) layoutFrame = requestAnimationFrame(measure);
    };
    const followHash = () => {
      // Keep old links after merging history and strength into one page.
      const id =
        location.hash === "#about" ? "strength" : location.hash.slice(1);
      if (location.hash === "#about")
        history.replaceState(
          history.state,
          "",
          `${location.pathname}${location.search}#strength`,
        );
      if (id && sections.some((section) => section.id === id)) {
        element
          .querySelector<HTMLElement>(`#${id}`)
          ?.scrollIntoView({ behavior: "instant", block: "start" });
      }
      schedule();
    };
    // This page is lazy-loaded; the initial hash can precede its DOM.
    measure();
    const initialFrame = requestAnimationFrame(followHash);
    window.addEventListener("scroll", schedule, { passive: true });
    window.addEventListener("resize", scheduleMeasure);
    window.addEventListener("hashchange", schedule);
    const observer = new ResizeObserver(scheduleMeasure);
    observer.observe(header);
    sections.forEach((section) => observer.observe(section));
    return () => {
      window.removeEventListener("scroll", schedule);
      window.removeEventListener("resize", scheduleMeasure);
      window.removeEventListener("hashchange", schedule);
      observer.disconnect();
      cancelAnimationFrame(frame);
      cancelAnimationFrame(layoutFrame);
      cancelAnimationFrame(initialFrame);
      if (previousHeight)
        html.style.setProperty("--landing-header-height", previousHeight);
      else html.style.removeProperty("--landing-header-height");
    };
  }, []);

  async function copyRecruitmentGroup() {
    try {
      await navigator.clipboard.writeText("674743797");
      setCopyState("copied");
    } catch {
      setCopyState("failed");
    }
  }

  return (
    <div className="public-home" ref={root}>
      <a className="skip-link" href="#main">
        {msg("跳转到主要内容")}
      </a>
      <div className="landing-topbar">
        <header className="landing-header landing-width">
          <a
            href="#home"
            className="landing-brand"
            aria-label={msg("GloryNavy 首页")}
            aria-current={activeSection === "home" ? "location" : undefined}
          >
            <img
              src="/images/glory-navy-logo.png"
              alt=""
              width="42"
              height="42"
            />
            <span>GloryNavy</span>
          </a>
          <nav className="landing-nav" aria-label={msg("军团介绍导航")}>
            {landingSections.slice(1).map(({ id, label }) => (
              <a
                key={id}
                href={`#${id}`}
                aria-current={activeSection === id ? "location" : undefined}
              >
                {msg(label)}
              </a>
            ))}
          </nav>
          <div className="landing-account">
            <LanguageSwitch />
            {session.data?.session ? (
              <Link className="landing-login" to="/workspace">
                {msg("进入工作台")}
                <ArrowUpRight size={16} aria-hidden="true" />
              </Link>
            ) : (
              <EveSignInButton compact />
            )}
          </div>
        </header>
      </div>
      <main id="main" tabIndex={-1}>
        <section
          id="home"
          className="landing-page landing-hero"
          aria-labelledby="landing-title"
        >
          {loginError && (
            <div className="landing-width landing-error-wrap">
              <p className="landing-login-error" role="alert">
                {loginError}
              </p>
            </div>
          )}
          <div className="landing-hero-body landing-width landing-page-content">
            <img
              className="landing-titan"
              src="/images/home-avatar.png"
              alt={msg("EVE Online 神使级泰坦")}
              width="1400"
              height="980"
              fetchPriority="high"
            />
            <div className="landing-hero-copy">
              <p className="landing-eyebrow">EVE ONLINE · TRANQUILITY</p>
              <h1 id="landing-title">GloryNavy</h1>
              <p className="landing-tagline">{msg("荣耀海军")}</p>
              <div className="landing-hero-actions">
                <a className="landing-primary" href="#join">
                  {msg("加入我们")}
                  <ArrowUpRight size={18} aria-hidden="true" />
                </a>
                <a className="landing-secondary" href="#strength">
                  {msg("探索军团")}
                  <ArrowDown size={16} aria-hidden="true" />
                </a>
              </div>
            </div>
          </div>
        </section>
        <section
          id="strength"
          className="landing-page landing-introduction"
          aria-labelledby="strength-title"
        >
          <div className="landing-width landing-page-content">
            <div className="landing-introduction-grid">
              <div className="landing-introduction-heading">
                <h2 id="strength-title">{msg("军团介绍")}</h2>
                <p className="landing-motto">{msg("荣耀海军（Glory Navy），EVE Online 宁静服玩家军团。")}</p>
                <img
                  className="landing-emblem"
                  src="/images/glory-navy-logo.png"
                  width="128"
                  height="128"
                  alt=""
                />
              </div>
              <dl className="landing-history">
                <div>
                  <dt>2016</dt>
                  <dd>{msg("晨曦启航，曾集结 80 余人的单军团舰队。")}</dd>
                </div>
                <div>
                  <dt>2021.08</dt>
                  <dd>{msg("宁静重建，专注 00 精英 PVP。")}</dd>
                </div>
              </dl>
            </div>
            <PublicStrength />
          </div>
        </section>
        <FleetChapters />
        <FleetChapters variant="support" />
        <section
          id="join"
          className="landing-page landing-join"
          aria-labelledby="join-title"
        >
          <div className="landing-width landing-page-content">
            <div className="landing-join-inner">
              <div>
                <h2 id="join-title">{msg("加入荣耀海军")}</h2>
                <p>{msg("欢迎新老舰长。")}</p>
              </div>
              <div className="landing-join-action">
                <div className="landing-group-number">
                  <span>{msg("QQ 招募群")}</span>
                  <strong>674743797</strong>
                </div>
                <div className="landing-contact-buttons">
                  <button
                    className="landing-primary"
                    onClick={() => void copyRecruitmentGroup()}
                  >
                    {copyState === "copied" ? (
                      <Check size={18} aria-hidden="true" />
                    ) : (
                      <Copy size={18} aria-hidden="true" />
                    )}
                    {copyState === "copied"
                      ? msg("已复制群号")
                      : msg("复制 QQ 群号")}
                  </button>
                  <a
                    className="landing-secondary"
                    href="https://kook.vip/h9CYhU"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {msg("加入 KOOK")}
                    <ArrowUpRight size={18} aria-hidden="true" />
                  </a>
                </div>
                <span
                  className={`landing-copy-status${copyState === "failed" ? "" : " sr-only"}`}
                  role="status"
                >
                  {copyState === "failed"
                    ? msg("复制失败，请手动复制：674743797")
                    : copyState === "copied"
                      ? msg("已复制群号")
                      : ""}
                </span>
              </div>
            </div>
            <JoiningSteps />
            <section
              className="landing-requirements"
              aria-labelledby="requirements-title"
            >
              <h3 id="requirements-title">{msg("入团要求")}</h3>
              <ul>
                <li>{msg("心智成熟、诚实守信，与军团成员友善相处。")}</li>
                <li>{msg("热爱 PVP，不畏惧 00、战斗与舰船损失。")}</li>
                <li>{msg("上线保持 Mumble 语音协作；现实不便时除外。")}</li>
                <li>
                  {msg("尊重队友和对手，不辱骂、不嘲讽，用战绩证明实力。")}
                </li>
                <li>
                  {msg(
                    "禁止脚本、外挂，以及通过非官方渠道购买 ISK、PLEX、技能点等物品。",
                  )}
                </li>
              </ul>
            </section>
            <footer className="landing-footer">
              <span>GloryNavy</span>
              <p>
                {msg("EVE Online 玩家军团网站。EVE 及舰船素材归 CCP 所有。")}
              </p>
            </footer>
          </div>
        </section>
      </main>
      <nav className="landing-pager" aria-label={msg("首页翻页")}>
        {activeIndex > 0 ? (
          <a
            href={`#${landingSections[activeIndex - 1].id}`}
            aria-label={msg("上一屏")}
          >
            <ArrowUp size={17} aria-hidden="true" />
          </a>
        ) : (
          <span className="landing-pager-placeholder" aria-hidden="true" />
        )}
        <span
          className="landing-page-number"
          aria-label={msg(
            "当前章节：{0}",
            msg(landingSections[activeIndex].label),
          )}
        >
          {String(activeIndex + 1).padStart(2, "0")}
          <span aria-hidden="true"> / 05</span>
        </span>
        {activeIndex < landingSections.length - 1 ? (
          <a
            href={`#${landingSections[activeIndex + 1].id}`}
            aria-label={msg("下一屏")}
          >
            <ArrowDown size={17} aria-hidden="true" />
          </a>
        ) : (
          <span className="landing-pager-placeholder" aria-hidden="true" />
        )}
      </nav>
    </div>
  );
}
