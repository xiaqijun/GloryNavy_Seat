import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { ArrowDown, ArrowUpRight, Check, Copy, Globe2 } from "lucide-react";
import gsap from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";
import { useGSAP } from "@gsap/react";
import { LanguageSwitch } from "@/components/language-switch";
import { useSession } from "@/modules/identity";
import { EveSignInButton } from "@/modules/eve/login-components";
import { useLoginError } from "@/modules/eve/use-login-error";
import { msg } from "@/lib/i18n";
import "./landing.css";
import {
  PublicStrength,
  FleetChapters,
  JoiningSteps,
} from "./landing-sections";

gsap.registerPlugin(ScrollTrigger, useGSAP);

const landingSections = [
  { id: "strength", label: "军团实力" },
  { id: "about", label: "关于我们" },
  { id: "fleet", label: "军团活动" },
  { id: "join", label: "加入我们" },
] as const;

export default function LandingPage() {
  const root = useRef<HTMLDivElement>(null);
  const [activeSection, setActiveSection] = useState("");
  const session = useSession();
  const loginError = useLoginError();
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">(
    "idle",
  );
  useEffect(() => {
    const title = document.title;
    document.title = msg("GloryNavy · 在新伊甸，一起远航");
    return () => {
      document.title = title;
    };
  }, []);
  useEffect(() => {
    const sections = landingSections.map(({ id }) =>
      root.current?.querySelector<HTMLElement>(`#${id}`),
    );
    let frame = 0;
    const update = () => {
      frame = 0;
      let current = "";
      for (const section of sections) {
        if (!section) continue;
        const offset =
          parseFloat(getComputedStyle(section).scrollMarginTop) || 0;
        if (section.getBoundingClientRect().top <= offset + 2)
          current = section.id;
      }
      // The final section may not reach the header on a tall viewport.
      if (
        window.scrollY > 0 &&
        window.scrollY + window.innerHeight >=
          document.documentElement.scrollHeight - 2
      ) {
        current = "join";
      }
      setActiveSection(current);
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(update);
    };
    window.addEventListener("scroll", schedule, { passive: true });
    window.addEventListener("resize", schedule);
    window.addEventListener("hashchange", schedule);
    const observer =
      typeof ResizeObserver === "undefined"
        ? undefined
        : new ResizeObserver(schedule);
    if (root.current) observer?.observe(root.current);
    schedule();
    return () => {
      window.removeEventListener("scroll", schedule);
      window.removeEventListener("resize", schedule);
      window.removeEventListener("hashchange", schedule);
      observer?.disconnect();
      cancelAnimationFrame(frame);
    };
  }, []);
  useGSAP(
    () => {
      const motion = gsap.matchMedia();
      motion.add("(prefers-reduced-motion: no-preference)", () => {
        gsap.from(".landing-hero-copy > *", {
          y: 18,
          opacity: 0,
          duration: 0.6,
          stagger: 0.09,
          ease: "power2.out",
        });
        gsap.to(".landing-titan", {
          y: () => (window.innerWidth <= 760 ? 24 : 80),
          scale: 1.04,
          ease: "none",
          scrollTrigger: {
            trigger: ".landing-hero",
            start: "top top",
            end: "bottom top",
            scrub: true,
            invalidateOnRefresh: true,
          },
        });
        gsap.utils
          .toArray<HTMLElement>(".landing-reveal")
          .forEach((element) => {
            gsap.from(element, {
              y: 48,
              opacity: 0,
              duration: 0.8,
              ease: "power2.out",
              scrollTrigger: {
                trigger: element,
                start: "top 90%",
                toggleActions: "play none none reverse",
              },
            });
          });
      });
      // Public data can change section height after reveal triggers are built.
      let refreshFrame = 0;
      const strength = root.current?.querySelector(".landing-strength");
      const observer =
        typeof ResizeObserver === "undefined"
          ? undefined
          : new ResizeObserver(() => {
              cancelAnimationFrame(refreshFrame);
              refreshFrame = requestAnimationFrame(() =>
                ScrollTrigger.refresh(),
              );
            });
      if (strength && observer) observer.observe(strength);
      return () => {
        observer?.disconnect();
        cancelAnimationFrame(refreshFrame);
        motion.revert();
      };
    },
    { scope: root },
  );

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
          <Link
            to="/"
            className="landing-brand"
            aria-label={msg("GloryNavy 首页")}
          >
            <img
              src="/images/glory-navy-logo.png"
              alt=""
              width="42"
              height="42"
            />
            <span>GloryNavy</span>
          </Link>
          <nav className="landing-nav" aria-label={msg("军团介绍导航")}>
            {landingSections.map(({ id, label }) => (
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
        <div className="landing-hero">
          {loginError && (
            <div className="landing-width landing-error-wrap">
              <p className="landing-login-error" role="alert">
                {loginError}
              </p>
            </div>
          )}
          <section
            className="landing-hero-body landing-width"
            aria-labelledby="landing-title"
          >
            <img
              className="landing-titan"
              src="/images/home-avatar.png"
              alt={msg("EVE Online 神使级泰坦")}
              width="1400"
              height="980"
              fetchPriority="high"
            />
            <div className="landing-hero-copy">
              <p className="landing-eyebrow">
                <span /> EVE ONLINE · TRANQUILITY
              </p>
              <h1 id="landing-title">GloryNavy</h1>
              <p className="landing-tagline">
                {msg("荣耀海军 · 00 精英 PVP 军团")}
              </p>
              <p className="landing-intro">
                {msg("从晨曦到宁静，欢迎热爱战斗的新老舰长。")}
              </p>
              <div className="landing-hero-actions">
                <a className="landing-primary" href="#join">
                  {msg("加入我们")}
                  <ArrowUpRight size={19} aria-hidden="true" />
                </a>
                <a className="landing-secondary" href="#strength">
                  {msg("军团实力")}
                  <ArrowDown size={16} aria-hidden="true" />
                </a>
              </div>
            </div>
            <div className="landing-hero-bottom">
              <a href="#strength">
                <ArrowDown size={16} aria-hidden="true" />
                {msg("探索军团")}
              </a>
              <span>{msg("神使级 · 泰坦")}</span>
            </div>
          </section>
        </div>

        <PublicStrength />

        <section
          id="about"
          className="landing-about landing-width"
          aria-labelledby="about-title"
        >
          <div className="landing-about-heading landing-reveal">
            <p className="landing-kicker">{msg("关于我们")}</p>
            <h2 id="about-title">
              {msg("荣耀海军，")}
              <br />
              {msg("全员猛男！")}
            </h2>
          </div>
          <div className="landing-about-copy landing-reveal">
            <dl className="landing-history">
              <div>
                <dt>{msg("2016 · 晨曦启航")}</dt>
                <dd>
                  {msg(
                    "成立于晨曦服的 00 主权战斗军团，曾组织单军团 80 余人的作战舰队。",
                  )}
                </dd>
              </div>
              <div>
                <dt>{msg("2021.08 · 宁静重建")}</dt>
                <dd>
                  {msg(
                    "转战宁静服，以精英 PVP 为方向，延续作战、生产与新人培养的军团传统。",
                  )}
                </dd>
              </div>
            </dl>
          </div>
        </section>

        <FleetChapters />
        <section
          id="join"
          className="landing-join"
          aria-labelledby="join-title"
        >
          <div className="landing-width landing-join-inner landing-reveal">
            <div>
              <p className="landing-kicker">{msg("加入我们")}</p>
              <h2 id="join-title">{msg("加入荣耀海军")}</h2>
              <p>
                {msg("欢迎新老玩家。申请入团，请先联系新人 QQ 群内管理员。")}
              </p>
            </div>
            <div className="landing-join-action">
              <span className="landing-group-number">
                {msg("QQ 招募群：674743797")}
              </span>
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
          <section
            className="landing-requirements landing-width"
            aria-labelledby="requirements-title"
          >
            <h3 id="requirements-title">{msg("入团要求")}</h3>
            <ul>
              <li>{msg("心智成熟、诚实守信，与军团成员友善相处。")}</li>
              <li>{msg("热爱 PVP，不畏惧 00、战斗与舰船损失。")}</li>
              <li>{msg("上线保持 Mumble 语音协作；现实不便时除外。")}</li>
              <li>{msg("尊重队友和对手，不辱骂、不嘲讽，用战绩证明实力。")}</li>
              <li>
                {msg(
                  "禁止脚本、外挂，以及通过非官方渠道购买 ISK、PLEX、技能点等物品。",
                )}
              </li>
            </ul>
          </section>
          <JoiningSteps />
        </section>
      </main>
      <div className="landing-site-footer">
        <footer className="landing-footer landing-width">
          <span>GloryNavy</span>
          <p>{msg("EVE Online 玩家军团网站。EVE 及舰船素材归 CCP 所有。")}</p>
          <span className="landing-world">
            <Globe2 size={13} aria-hidden="true" />
            Tranquility
          </span>
        </footer>
      </div>
    </div>
  );
}
