import { msg } from "@/lib/i18n";
import { useQuery } from "@tanstack/react-query";
import { Suspense, useEffect, useRef } from "react";
import { House, Shield, Globe2, RefreshCw } from "lucide-react";
import { Link, Navigate, Route, Routes, useLocation } from "react-router-dom";
import { IconAction } from "@/components/ui/icon-action";
import { getModuleCatalog } from "@/app/catalog";
import { selectPages } from "@/app/module-registry";
import { frontendModules } from "@/app/modules";
import { SidebarNavigation } from "@/app/sidebar-navigation";
import { PageBoundary } from "@/app/page-boundary";
import { AccountEntry, useSession } from "@/modules/identity";
import { useManagementAccess } from "@/modules/access";
import { LanguageSwitch } from "@/components/language-switch";
import { getContext as getApprovalContext } from "@/modules/approval/api";
import { warmNavigationPages } from "@/app/navigation-warmup";
import { queryClient } from "@/lib/query";

export default function MemberApp() {
  const memberSession = useSession(true);
  const catalog = useQuery({
    queryKey: ["host", "modules"],
    enabled: !!memberSession.data?.session,
    queryFn: ({ signal }) => getModuleCatalog(signal),
    select: (active) => selectPages(frontendModules, active),
  });
  const pages = catalog.data ?? [];
  // Module discovery and access are independent authenticated reads. Start
  // them together so the first protected page does not pay two network RTTs.
  const management = useManagementAccess(!!memberSession.data?.session);
  const location = useLocation();
  const approvalSession = useSession(
    pages.some((p) => p.permission === "approval.self"),
  );
  const approvalAccess = useQuery({
    queryKey: ["approval", "context", approvalSession.data?.session?.user_id],
    queryFn: ({ signal }) => getApprovalContext(signal),
    enabled:
      pages.some((p) => p.permission === "approval.self") &&
      !!approvalSession.data?.session,
    refetchInterval: 30000,
  });
  const canOpen = (page: (typeof pages)[number]) =>
    (!page.administratorOnly ||
      (management.access.isSuccess &&
        management.access.data.administrator === true)) &&
    (!page.permission ||
      (page.permission === "approval.self"
        ? approvalAccess.isSuccess && approvalAccess.data.allowed === true
        : page.permission === "access.members.read"
          ? management.access.isSuccess &&
            management.access.data.administrator === true
          : page.permission === "eve.sync.manage"
            ? management.access.isSuccess &&
              (management.access.data.administrator === true ||
                management.access.data.can_manage_sync === true)
            : management.allowed));
  const visiblePages = pages.filter(canOpen);
  const visibleNavigationIds = visiblePages
    .filter((page) => page.navigation !== false)
    .map((page) => page.id)
    .join("|");
  const administratorPage = pages.find(
    (page) => page.path === location.pathname && page.administratorOnly,
  );
  const currentPage = visiblePages.find(
    (page) => page.path === location.pathname,
  );
  const currentPageVisible = !!currentPage;
  // Only load a route after the session and its page-level access are known.
  useEffect(() => {
    if (memberSession.data?.session && currentPage)
      void currentPage.preload().catch(() => {});
  }, [memberSession.data?.session, currentPage]);
  useEffect(() => {
    if (!memberSession.data?.session || !visibleNavigationIds) return;
    return warmNavigationPages(
      visiblePages,
      location.pathname,
      () => queryClient.isFetching() > 0,
    );
    // The authorization-derived page IDs are the warmup queue's identity.
    // Route changes must not restart a queue that is already warming.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [memberSession.data?.session?.user_id, visibleNavigationIds]);
  const guardHome =
    location.pathname === "/workspace" &&
    pages.some((page) => page.id === "eve.login");
  const homeSession = useSession(guardHome);
  const standalone =
    (guardHome && !homeSession.data?.session) ||
    frontendModules
      .flatMap((m) => m.pages)
      .some((p) => p.path === location.pathname && p.layout === "standalone");
  const previousPath = useRef(location.pathname);
  useEffect(() => {
    if (previousPath.current === location.pathname) return;
    previousPath.current = location.pathname;
    document.getElementById("main")?.focus({ preventScroll: true });
    window.scrollTo({ top: 0, behavior: "instant" });
  }, [location.pathname]);
  if (memberSession.isError)
    return <p role="alert">{memberSession.error.message}</p>;
  if (!memberSession.data) return <p role="status">{msg("正在加载")}</p>;
  if (!memberSession.data.session) return <Navigate to="/login" replace />;
  return (
    <div className={standalone ? "auth-shell" : "app-shell"}>
      <a className="skip-link" href="#main">
        {msg("跳转到主要内容")}{" "}
      </a>
      {!standalone && (
        <aside className="sidebar">
          <Link to="/" className="brand" aria-label={msg("GloryNavy 首页")}>
            <span className="brand-mark">
              <Shield size={23} aria-hidden="true" />
            </span>
            <span>
              GloryNavy<small>{msg("军团管理平台")}</small>
            </span>
          </Link>
          <SidebarNavigation
            pathname={location.pathname}
            pages={visiblePages.filter((page) => page.navigation !== false)}
          />
        </aside>
      )}
      <div className={standalone ? "auth-main-shell" : "main-shell"}>
        {standalone && (
          <div className="auth-language">
            <LanguageSwitch />
          </div>
        )}
        {!standalone && (
          <header className="topbar">
            <span className="environment-tag">
              <Globe2 size={14} aria-hidden="true" />
              {msg("国际服 · Tranquility")}{" "}
            </span>
            <div className="topbar-actions">
              <LanguageSwitch />
              {pages.some((page) => page.id === "eve.login") && (
                <AccountEntry />
              )}
            </div>
          </header>
        )}
        <main id="main" tabIndex={-1}>
          {catalog.isPending ? (
            <p role="status" className="muted">
              {msg("正在加载")}{" "}
            </p>
          ) : catalog.isError ? (
            <div className="error-panel" role="alert">
              <p>{msg("页面配置暂不可用，请重试。")}</p>
              <IconAction
                label={msg("重新加载页面配置")}
                onClick={() => void catalog.refetch()}
              >
                <RefreshCw aria-hidden="true" />
              </IconAction>
            </div>
          ) : guardHome && homeSession.isPending ? (
            <p role="status" className="muted">
              {msg("正在检查登录状态")}{" "}
            </p>
          ) : guardHome && homeSession.isError ? (
            <div className="error-panel" role="alert">
              <p>{msg("登录状态暂不可用，请重试。")}</p>
              <IconAction
                label={msg("重试登录状态")}
                onClick={() => void homeSession.refetch()}
              >
                <RefreshCw aria-hidden="true" />
              </IconAction>
            </div>
          ) : guardHome && !homeSession.data?.session ? (
            <Navigate to="/login" replace />
          ) : administratorPage &&
            !currentPageVisible &&
            management.access.isPending ? (
            <p role="status" className="muted">
              {msg("正在加载")}
            </p>
          ) : administratorPage &&
            !currentPageVisible &&
            management.access.isError ? (
            <div className="error-panel" role="alert">
              <p>{management.access.error.message}</p>
              <IconAction
                label={msg("重试")}
                onClick={() => void management.access.refetch()}
              >
                <RefreshCw aria-hidden="true" />
              </IconAction>
            </div>
          ) : (
            <PageBoundary key={location.pathname}>
              <Suspense
                fallback={
                  <p role="status" className="muted">
                    {msg("正在加载")}{" "}
                  </p>
                }
              >
                <Routes>
                  {pages
                    .filter((page) => !page.administratorOnly || canOpen(page))
                    .map(({ id, path, component: Page }) => (
                      <Route key={id} path={path} element={<Page />} />
                    ))}
                  <Route
                    path="*"
                    element={
                      <div className="empty-page">
                        <h1>{msg("页面不存在")}</h1>
                        <IconAction label={msg("返回工作台")} asChild>
                          <Link to="/workspace">
                            <House aria-hidden="true" />
                          </Link>
                        </IconAction>
                      </div>
                    }
                  />
                </Routes>
              </Suspense>
            </PageBoundary>
          )}
        </main>
      </div>
    </div>
  );
}

