import { msg } from "@/lib/i18n";
import { lazy, Suspense, useEffect } from "react";
import { useLocation } from "react-router-dom";
import { PageBoundary } from "@/app/page-boundary";
import PublicHome from "@/modules/system/landing-page";
import { usePageMetadata } from "@/app/page-metadata";

// Only the reviewed, local introduction is public. Do not fetch the private
// module catalog or management context before entering the member application.
const MemberApp = lazy(() => import("@/app/member-app"));

export default function App() {
  const location = useLocation();
  usePageMetadata(location.pathname);
  useEffect(() => {
    document.documentElement.dataset.appReady = "true";
    return () => {
      delete document.documentElement.dataset.appReady;
    };
  }, []);
  if (location.pathname === "/") {
    return (
      <PageBoundary>
        <PublicHome />
      </PageBoundary>
    );
  }
  return (
    <PageBoundary>
      <Suspense fallback={<p role="status">{msg("正在加载")}</p>}>
        <MemberApp />
      </Suspense>
    </PageBoundary>
  );
}
