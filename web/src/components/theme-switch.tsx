import { Monitor, Moon, Sun } from "lucide-react";
import { Select } from "@/components/ui/select";
import { msg } from "@/lib/i18n";
import { setThemePreference, useThemePreference } from "@/lib/theme";

export function ThemeSwitch() {
  const value = useThemePreference();
  return <Select label={msg("主题")} value={value}
    onValueChange={setThemePreference} className="theme-switch"
    options={[
      { value: "system", label: msg("跟随系统"), leading: <Monitor size={18} aria-hidden="true" /> },
      { value: "light", label: msg("浅色"), leading: <Sun size={18} aria-hidden="true" /> },
      { value: "dark", label: msg("暗色"), leading: <Moon size={18} aria-hidden="true" /> },
    ]} />;
}
