import { Select } from "@/components/ui/select";
import { msg } from "@/lib/i18n";
import type { Character } from "./api";

export function selectedOfficer(officers: Character[], value: string) {
  return officers.find(c => c.id === value) ?? officers.find(c => c.is_main) ?? officers[0];
}

export function LoanOfficerSelect({ officers, value, onChange }: { officers: Character[]; value: string; onChange: (value: string) => void }) {
  return <div className="loan-officer-select"><span>{msg("贷款专员")}</span><Select
    label={msg("贷款专员")} value={selectedOfficer(officers, value)?.id ?? ""} onValueChange={onChange}
    disabled={!officers.length} placeholder={msg("暂无可用贷款专员")}
    options={officers.map(c => ({ value: c.id, label: c.is_main ? msg("{0}（主角色）", c.name) : c.name }))}
  /></div>;
}
