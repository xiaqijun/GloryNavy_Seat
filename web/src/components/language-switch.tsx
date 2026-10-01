import { Languages } from "lucide-react";
import { Button } from "@/components/ui/button";
import { changeLocale, getLocale, msg } from "@/lib/i18n";

export function LanguageSwitch() {
  const next = getLocale() === "zh-CN" ? "en" : "zh-CN";
  const label = msg(next === "en" ? "切换为英文" : "切换为中文");
  return (
    <Button
      type="button"
      variant="ghost"
      className="language-switch"
      title={label}
      aria-label={label}
      onClick={() => changeLocale(next)}
    >
      <Languages size={18} aria-hidden="true" />
      <span lang={next}>{next === "en" ? "EN" : "中文"}</span>
    </Button>
  );
}
