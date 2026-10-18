import { msg } from "@/lib/i18n";
import { useEffect, useRef, useState } from "react";
import { NavLink } from "react-router-dom";
import {
  ChevronDown,
  Crosshair,
  Landmark,
  Gift,
  Settings2,
  Menu,
  UsersRound,
} from "lucide-react";
import type { RegisteredModule } from "./module-registry";
import "./sidebar-navigation.css";

type Page = RegisteredModule["pages"][number];
const groups = [
  { id: "corporation", label: msg("军团事务"), icon: UsersRound },
  { id: "operations", label: msg("作战训练"), icon: Crosshair },
  { id: "finance", label: msg("资产服务"), icon: Landmark },
  { id: "benefits", label: msg("福利兑换"), icon: Gift },
  { id: "administration", label: msg("系统管理"), icon: Settings2 },
] as const;
export function SidebarNavigation({
  pages,
  pathname,
}: {
  pages: Page[];
  pathname: string;
}) {
  const orderedPages = [...pages].sort(
    (a, b) => (a.navigationOrder ?? 100) - (b.navigationOrder ?? 100),
  );
  const footerPages = orderedPages.filter((page) => page.navigationFooter);
  const primaryPages = orderedPages.filter((page) => !page.navigationFooter);
  const active = pages.find((p) => p.path === pathname)?.navigationGroup;
  // Permissions can arrive after the route. Until the user chooses a group,
  // derive it from the current available page instead of freezing an empty value.
  const [expanded, setExpanded] = useState<string | null>(null);
  const [mobileOpen, setMobileOpen] = useState(false);
  const toggleRef = useRef<HTMLButtonElement>(null);
  const preloadTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const cancelPreload = () => {
    if (preloadTimer.current !== null) clearTimeout(preloadTimer.current);
    preloadTimer.current = null;
  };
  useEffect(() => cancelPreload, []);
  const schedulePreload = (preload: Page["preload"]) => {
    cancelPreload();
    preloadTimer.current = setTimeout(() => {
      preloadTimer.current = null;
      void preload().catch(() => {});
    }, 180);
  };
  const link = ({ id, path, label, icon: Icon, preload }: Page) => (
    <NavLink
      key={id}
      to={path}
      end
      onPointerEnter={() => schedulePreload(preload)}
      onPointerLeave={cancelPreload}
      onPointerDown={() => {
        cancelPreload();
        void preload().catch(() => {});
      }}
      onFocus={() => schedulePreload(preload)}
      onBlur={cancelPreload}
      onKeyDown={(event) => {
        if (event.key === "Enter") {
          cancelPreload();
          void preload().catch(() => {});
        }
      }}
      onClick={() => {
        cancelPreload();
        setMobileOpen(false);
      }}
    >
      <Icon size={18} aria-hidden="true" />
      <span>{label}</span>
    </NavLink>
  );
  return (
    <>
      <button
        type="button"
        ref={toggleRef}
        className="navigation-toggle"
        aria-label={msg("菜单")}
        aria-expanded={mobileOpen}
        aria-controls="workspace-navigation"
        onClick={() => setMobileOpen(!mobileOpen)}
        onKeyDown={(event) => {
          if (event.key === "Escape") setMobileOpen(false);
        }}
      >
        <Menu size={20} aria-hidden="true" />
        <span>{msg("菜单")}</span>
      </button>
      <nav
        id="workspace-navigation"
        className="workspace-navigation"
        aria-label={msg("主导航")}
        data-mobile-open={mobileOpen}
        onKeyDown={(event) => {
          if (
            event.key === "Escape" &&
            mobileOpen &&
            toggleRef.current?.getClientRects().length
          ) {
            setMobileOpen(false);
            toggleRef.current.focus();
          }
        }}
      >
        <div className="navigation-primary">
          {primaryPages.filter((p) => !p.navigationGroup).map(link)}
          {groups.map(({ id, label, icon: Icon }) => {
            const children = primaryPages.filter(
              (p) => p.navigationGroup === id,
            );
            if (!children.length) return null;
            const open = (expanded ?? active) === id;
            return (
              <div className="navigation-group" key={id}>
                <button
                  type="button"
                  className="navigation-group-trigger"
                  data-active={active === id}
                  aria-expanded={open}
                  aria-controls={`navigation-${id}`}
                  onClick={() => setExpanded(open ? "" : id)}
                >
                  <Icon size={18} aria-hidden="true" />
                  <span>{label}</span>
                  <ChevronDown
                    size={16}
                    aria-hidden="true"
                    className={open ? "is-expanded" : ""}
                  />
                </button>
                <div
                  id={`navigation-${id}`}
                  className="navigation-children"
                  hidden={!open}
                >
                  {children.map(link)}
                </div>
              </div>
            );
          })}
        </div>
        {footerPages.length > 0 && (
          <div className="navigation-footer">{footerPages.map(link)}</div>
        )}
      </nav>
    </>
  );
}
