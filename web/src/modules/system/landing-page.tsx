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

export default function LandingPage() {
  const root = useRef<HTMLDivElement>(null);
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
          y: 100,
          scale: 1.07,
          ease: "none",
          scrollTrigger: {
            trigger: ".landing-hero",
            start: "top top",
            end: "bottom top",
            scrub: true,
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
      motion.add(
        "(min-width: 1000px) and (min-height: 650px) and (prefers-reduced-motion: no-preference)",
        () => {
          const panels = gsap.utils.toArray<HTMLElement>(
            ".landing-chapter-panel",
          );
          const markers = gsap.utils.toArray<HTMLElement>(
            ".landing-chapter-progress span",
          );
          gsap.set(".landing-chapter-panels", {
            display: "grid",
          });
          gsap.set(panels, {
            gridArea: "1 / 1",
          });
          gsap.set(panels.slice(1), {
            autoAlpha: 0,
            y: 60,
          });
          gsap.set(markers.slice(1), { opacity: 0.22 });
          const story = gsap.timeline({
            scrollTrigger: {
              trigger: ".landing-chapters",
              start: "top top",
              end: "+=170%",
              pin: true,
              scrub: 0.6,
              invalidateOnRefresh: true,
            },
          });
          story.to(
            ".landing-chapter-art",
            { scale: 1.13, xPercent: 5, duration: 5, ease: "none" },
            0,
          );
          panels.slice(1).forEach((panel, index) => {
            const at = 1 + index * 2;
            story
              .to(panels[index], { autoAlpha: 0, y: -50, duration: 0.5 }, at)
              .to(panel, { autoAlpha: 1, y: 0, duration: 0.6 }, at + 0.2)
              .to(markers[index], { opacity: 0.22, duration: 0.4 }, at)
              .to(markers[index + 1], { opacity: 1, duration: 0.4 }, at + 0.45);
          });
        },
      );
      // Public data arrives after the timeline is built. Re-measure downstream
      // pin/reveal offsets when that section's layout changes.
      let refreshFrame = 0;
      const strength = root.current?.querySelector(".landing-strength");
      const observer =
        typeof ResizeObserver === "undefined"
          ? undefined
          : new ResizeObserver(() => {
              cancelAnimationFrame(refreshFrame);
              refreshFrame = requestAnimationFrame(() => ScrollTrigger.refresh());
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
      <div className="landing-hero">
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
            <a href="#about">{msg("关于我们")}</a>
            <a href="#strength">{msg("军团实力")}</a>
            <a href="#fleet">{msg("舰队生活")}</a>
            <a href="#join">{msg("加入我们")}</a>
          </nav>
          <div className="landing-account">
            <LanguageSwitch />
            {loginError && (
              <p className="landing-login-error" role="alert">
                {loginError}
              </p>
            )}
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
        <main id="main" tabIndex={-1}>
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
              <p className="landing-tagline">{msg("在新伊甸，一起远航。")}</p>
              <p className="landing-intro">
                {msg("从第一次跃迁，到并肩出征。下一段故事，与你一起。")}
              </p>
              <a className="landing-primary" href="#join">
                {msg("加入我们")}
                <ArrowUpRight size={19} aria-hidden="true" />
              </a>
            </div>
            <div className="landing-hero-bottom">
              <a href="#about">
                <ArrowDown size={16} aria-hidden="true" />
                {msg("探索军团")}
              </a>
              <span>{msg("神使级 · 泰坦")}</span>
            </div>
          </section>
        </main>
      </div>

      <section
        id="about"
        className="landing-about landing-width"
        aria-labelledby="about-title"
      >
        <div className="landing-about-heading landing-reveal">
          <p className="landing-kicker">01 / {msg("关于我们")}</p>
          <h2 id="about-title">
            {msg("星海很大，")}
            <br />
            {msg("一起走得更远。")}
          </h2>
          <p>
            {msg(
              "Glory Navy 是 EVE Online 国际服军团。我们因新伊甸相遇，在舰队中找到伙伴，也为每一次出发做好准备。",
            )}
          </p>
          <div id="life" className="landing-life">
            {[
              [
                "01",
                "并肩出征",
                "从集结到交战，与伙伴协作，在新伊甸留下属于我们的航迹。",
              ],
              [
                "02",
                "共同成长",
                "交流配装，规划技能。在探索自己的飞行方式时，也有人与你同行。",
              ],
            ].map(([number, title, description]) => (
              <article
                className="landing-life-item landing-reveal"
                key={number}
              >
                <span className="landing-number" aria-hidden="true">
                  {number}
                </span>
                <div>
                  <h3>{msg(title)}</h3>
                  <p>{msg(description)}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
        <div className="landing-about-art landing-reveal" aria-hidden="true">
          <img
            src="/images/home-avatar.png"
            alt=""
            width="1400"
            height="980"
            loading="lazy"
          />
        </div>
      </section>

      <PublicStrength />
      <FleetChapters />
      <JoiningSteps />
      <section id="join" className="landing-join" aria-labelledby="join-title">
        <div className="landing-width landing-join-inner landing-reveal">
          <div>
            <p className="landing-kicker">05 / {msg("加入我们")}</p>
            <h2 id="join-title">{msg("下一次跃迁，一起。")}</h2>
            <p>{msg("加入招募群，认识下一次并肩出征的伙伴。")}</p>
          </div>
          <div className="landing-join-action">
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
            <span className="landing-copy-status" role="status">
              {copyState === "failed"
                ? msg("复制失败，请手动复制：674743797")
                : msg("QQ 招募群：674743797")}
            </span>
          </div>
        </div>
        <footer className="landing-footer landing-width">
          <span>GloryNavy</span>
          <p>{msg("EVE Online 玩家军团网站。EVE 及舰船素材归 CCP 所有。")}</p>
          <span className="landing-world">
            <Globe2 size={13} aria-hidden="true" />
            Tranquility
          </span>
        </footer>
      </section>
    </div>
  );
}
