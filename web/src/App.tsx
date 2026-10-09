import { msg } from "@/lib/i18n";
import { lazy, Suspense } from "react";
import { useLocation } from "react-router-dom";
import { PageBoundary } from "@/app/page-boundary";
import { systemModule } from "@/modules/system";

// Only the reviewed, local introduction is public. Do not fetch the private
// module catalog or management context before entering the member application.
const PublicHome = lazy(
  systemModule.pages.find((page) => page.id === "system.landing")!.load,
);
const MemberApp = lazy(() => import("@/app/member-app"));

export default function App() {
  const location = useLocation();
  if (location.pathname === "/") {
    return (
      <PageBoundary>
        <Suspense fallback={<p role="status">{msg("正在加载")}</p>}>
          <PublicHome />
        </Suspense>
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
