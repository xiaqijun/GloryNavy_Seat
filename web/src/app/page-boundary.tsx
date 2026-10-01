import { msg } from "@/lib/i18n";
import { Component, type ReactNode } from "react";
import { RefreshCw } from "lucide-react";
import { IconAction } from "@/components/ui/icon-action";

export class PageBoundary extends Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <div className="error-panel" role="alert">
        <p>{msg("页面加载失败，请刷新重试。")}</p>
        <IconAction
          label={msg("重新加载页面")}
          onClick={() => window.location.reload()}
        >
          <RefreshCw aria-hidden="true" />
        </IconAction>
      </div>
    );
  }
}
