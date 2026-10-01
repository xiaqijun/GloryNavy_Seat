import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Camera, Check, FileText, Plus, Search, Settings2, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Modal } from "@/components/ui/dialog";
import { Select } from "@/components/ui/select";
import { RewardSummary } from "@/components/reward-summary";
import { getCatalog } from "@/modules/exchange/catalog-api";
import { amount as contractAmount, getContracts, title as contractTitle, type Contract } from "@/modules/eve/contracts-api";
import { APIError, apiFetch } from "@/lib/http";
import { msg } from "@/lib/i18n";
import * as api from "./api";

export function ActivityPanel({ corp, csrf, policies, characters, administrator, done }: {
  corp: string; csrf: string; policies: api.Policy[]; characters: api.Character[];
  administrator: boolean; done: () => void;
}) {
  const [editing, setEditing] = useState<api.Policy | "new" | null>(null);
  const [applying, setApplying] = useState<api.Policy | null>(null);
  const available = policies.filter((p) => p.kind.startsWith("activity_") && (p.config.enabled || administrator));
  return <section className="welfare-activity-section" aria-label={msg("活动项目")}>
    <div className="welfare-activity-heading"><h2>{msg("活动项目")}</h2>
      {administrator && <Button variant="outline" onClick={() => setEditing("new")}><Plus size={16} />{msg("新增项目")}</Button>}
    </div>
    {available.length ? <div className="welfare-activity-grid">{available.map((p) => <Card key={p.kind} className="welfare-activity-card">
      <div className="welfare-activity-card-title"><strong>{p.config.project_name}</strong>
        {administrator && <Button variant="outline" aria-label={msg("编辑{0}", p.config.project_name)} onClick={() => setEditing(p)}><Settings2 size={16} /></Button>}
      </div>
      <RewardSummary rewards={p.config.rewards} compact />
      <Button disabled={!p.config.enabled || !characters.length} onClick={() => setApplying(p)}>{msg("申请")}</Button>
    </Card>)}</div> : <Card>{msg("暂无活动项目")}</Card>}
    {editing && <ProjectEditor corp={corp} csrf={csrf} current={editing === "new" ? undefined : editing} close={() => setEditing(null)} done={() => { setEditing(null); done(); }} />}
    {applying && <ApplicationEditor corp={corp} csrf={csrf} project={applying} characters={characters} close={() => setApplying(null)} done={() => { setApplying(null); done(); }} />}
  </section>;
}

function ProjectEditor({ corp, csrf, current, close, done }: { corp: string; csrf: string; current?: api.Policy; close: () => void; done: () => void }) {
  const [name, setName] = useState(current?.config.project_name || "");
  const [rewardID, setRewardID] = useState(current?.config.reward_id || "");
  const [coins, setCoins] = useState(String((current?.config.rewards?.coins_minor || 0) / 100));
  const [claimLimit, setClaimLimit] = useState(current?.config.claim_limit === undefined ? "0" : String(current.config.claim_limit));
  const [enabled, setEnabled] = useState(current?.config.enabled ?? true);
  const [busy, setBusy] = useState(false), [error, setError] = useState("");
  const catalog = useQuery({ queryKey: ["catalog", "all"], queryFn: ({ signal }) => getCatalog(signal) });
  const choices = (catalog.data || []).filter((r) => (!r.archived || r.id === current?.config.reward_id) && r.content.fittings.every((f) => f.corporation_id === corp));
  const selected = choices.find((r) => r.id === rewardID);
  const rewardOptions = [{ value: "", label: msg("仅果壳币") }, ...choices.map((r) => ({ value: r.id, label: r.name }))];
  if (rewardID && !selected) rewardOptions.push({ value: rewardID, label: catalog.isPending ? msg("正在加载奖励") : msg("奖励 #{0} 不可用", rewardID) });
  const valid = !!name.trim() && name.trim().length <= 100 && (!rewardID || !!selected) && /^\d+(\.\d{1,2})?$/.test(coins) && (Number(coins) > 0 || !!selected) && /^\d+$/.test(claimLimit) && Number(claimLimit) <= 1000;
  return <Modal title={current ? msg("项目配置") : msg("新增活动项目")} close={close} busy={busy} footer={<><Button variant="outline" onClick={close} disabled={busy}>{msg("取消")}</Button><Button type="submit" form="activity-project-form" disabled={busy || !valid}>{busy ? msg("正在保存") : msg("保存")}</Button></>}>
    <form id="activity-project-form" className="welfare-form welfare-activity-form" onSubmit={async (event) => {
      event.preventDefault(); if (!valid) return; setBusy(true); setError("");
      try { await api.post("activity/projects", csrf, { corporation_id: corp, project_id: current?.kind.slice(9) || "0", version: current?.version || "0", request_key: crypto.randomUUID(), name: name.trim(), enabled, reward_id: selected?.id || "0", reward_version: selected?.version || "0", coins_minor: api.minor(coins), claim_limit: Number(claimLimit) }); done(); }
      catch (e) { setError(e instanceof Error ? e.message : msg("操作失败")); }
      finally { setBusy(false); }
    }}>
      <label className="welfare-field">{msg("项目名称")}<input autoFocus required maxLength={100} value={name} onChange={(e) => setName(e.target.value)} /></label>
      <div className="welfare-activity-fields">
        <div className="welfare-field"><span>{msg("奖励库")}</span><Select label={msg("奖励库")} value={rewardID} onValueChange={setRewardID} options={rewardOptions} disabled={catalog.isPending} /></div>
        <label className="welfare-field">{msg("果壳币数量")}<input type="number" min="0" step="0.01" value={coins} onChange={(e) => setCoins(e.target.value)} /></label>
      </div>
      <label className="welfare-field">{msg("每个角色最多领取次数")}<input type="number" min="0" max="1000" step="1" value={claimLimit} onChange={(e) => setClaimLimit(e.target.value)} /><small className="welfare-field-hint">{msg("填 0 表示不限制")}</small></label>
      <label className="welfare-check"><input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />{msg("开放申请")}</label>
      {selected && <div className="welfare-activity-preview"><span>{msg("发放内容")}</span><RewardSummary rewards={{ ...selected.content, coins_minor: /^\d+(\.\d{1,2})?$/.test(coins) ? api.minor(coins) : 0 }} compact /></div>}
      {(error || catalog.isError) && <p role="alert">{error || catalog.error?.message}</p>}
    </form>
  </Modal>;
}

function ApplicationEditor({ corp, csrf, project, characters, close, done }: { corp: string; csrf: string; project: api.Policy; characters: api.Character[]; close: () => void; done: () => void }) {
  const [selectedCharacters, setSelectedCharacters] = useState<string[]>(characters[0] ? [characters[0].id] : []);
  const [description, setDescription] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [previews, setPreviews] = useState<{ file: File; url: string }[]>([]);
  const [contractCharacterID, setContractCharacterID] = useState(characters[0]?.id || "");
  const [contractID, setContractID] = useState("");
  const [contractSearch, setContractSearch] = useState("");
  const [contractQuery, setContractQuery] = useState("");
  const [showContracts, setShowContracts] = useState(false);
  const [busy, setBusy] = useState(false), [error, setError] = useState("");
  const validFiles = files.length >= 1 && files.length <= 3 && files.every((f) => f.size > 0 && f.size <= 2 * 1024 * 1024 && ["image/jpeg", "image/png", "image/webp"].includes(f.type));
  const allSelected = characters.length > 0 && selectedCharacters.length === characters.length;
  const toggleCharacter = (id: string) => setSelectedCharacters((current) => current.includes(id) ? current.filter((value) => value !== id) : [...current, id]);
  useEffect(() => {
    const next = files.map((file) => ({ file, url: URL.createObjectURL(file) }));
    setPreviews(next);
    return () => next.forEach(({ url }) => URL.revokeObjectURL(url));
  }, [files]);
  useEffect(() => {
    if (!selectedCharacters.includes(contractCharacterID)) {
      setContractCharacterID(selectedCharacters[0] || "");
      setContractID("");
    }
  }, [contractCharacterID, selectedCharacters]);
  const contractCharacter = characters.find((character) => character.id === contractCharacterID);
  const contractList = useQuery({
    queryKey: ["activity", "contracts", contractCharacterID, contractQuery],
    queryFn: ({ signal }) => getContracts({ kind: "character", id: contractCharacterID, name: contractCharacter?.name || "" }, new URLSearchParams({ q: contractQuery }), signal),
    enabled: showContracts && !!contractCharacterID,
  });
  const contractItems = contractList.data?.items || [];
  return <Modal title={project.config.project_name || msg("活动福利申请")} close={close} busy={busy} footer={<><Button variant="outline" onClick={close} disabled={busy}>{msg("取消")}</Button><Button type="submit" form="activity-application-form" disabled={busy || !selectedCharacters.length || !validFiles}>{busy ? msg("正在提交") : msg("提交申请")}</Button></>}>
    <form id="activity-application-form" className="welfare-form welfare-activity-form" onSubmit={async (event) => {
      event.preventDefault(); if (!validFiles) return; setBusy(true); setError("");
      try {
        const form = new FormData(); form.set("corporation_id", corp); form.set("project_id", project.kind.slice(9)); selectedCharacters.forEach((id) => form.append("character_id", id)); if (description.trim()) form.set("description", description.trim()); if (contractID) form.set("contract_id", contractID); form.set("request_key", crypto.randomUUID()); files.forEach((file) => form.append("images", file));
        const response = await apiFetch("/api/v1/welfare/activity/applications", { method: "POST", credentials: "same-origin", headers: { "X-CSRF-Token": csrf }, body: form });
        const payload = await response.json().catch(() => null);
        if (!response.ok) throw new APIError(payload?.error?.message || msg("提交失败"), response.status, payload?.request_id);
        done();
      } catch (e) { setError(e instanceof Error ? e.message : msg("操作失败")); }
      finally { setBusy(false); }
    }}>
      <div className="welfare-activity-preview"><span>{msg("发放内容")}</span><RewardSummary rewards={project.config.rewards} compact /></div>
      <fieldset className="welfare-field welfare-character-picker">
        <legend>{msg("领取角色")}</legend>
        <div className="welfare-character-picker-head"><span className="welfare-character-picker-count">{msg("已选 {0} 个", selectedCharacters.length)}</span><div className="welfare-character-picker-actions"><Button type="button" variant="ghost" size="sm" onClick={() => setSelectedCharacters(allSelected ? [] : characters.map((c) => c.id))}>{allSelected ? msg("清空") : msg("全选")}</Button></div></div>
        <div className="welfare-character-picker-list">
          {characters.map((character) => { const selected = selectedCharacters.includes(character.id); return <label key={character.id} className={`welfare-character-option${selected ? " is-selected" : ""}`}><input type="checkbox" checked={selected} onChange={() => toggleCharacter(character.id)} /><span className="welfare-character-option-mark">{selected && <Check size={14} aria-hidden="true" />}</span><span className="welfare-character-option-name">{character.name}</span>{character.main_character_name && character.main_character_name !== character.name && <small>{character.main_character_name}</small>}</label>; })}
        </div>
      </fieldset>
      <label className="welfare-field"><span>{msg("申请说明")} <small className="welfare-field-hint">{msg("可选")}</small></span><textarea autoFocus maxLength={1000} value={description} onChange={(e) => setDescription(e.target.value)} /></label>
      <div className="welfare-activity-contract-field">
        <button type="button" className={`welfare-activity-contract-toggle${showContracts ? " is-open" : ""}`} onClick={() => setShowContracts((value) => !value)} aria-expanded={showContracts}><FileText size={16} aria-hidden="true" /><span>{msg("选择合同材料")}</span><small>{contractID ? msg("已选择合同 #{0}", contractID) : msg("可选")}</small></button>
        {showContracts && <div className="welfare-activity-contract-picker">
          <div className="welfare-activity-contract-toolbar">
            <Select label={msg("合同所属角色")} value={contractCharacterID} onValueChange={(value) => { setContractCharacterID(value); setContractID(""); }} options={characters.map((character) => ({ value: character.id, label: character.name }))} />
            <div className="welfare-activity-contract-search"><input aria-label={msg("搜索合同")} placeholder={msg("搜索合同描述或 ID")} value={contractSearch} onChange={(event) => setContractSearch(event.target.value)} /><Button type="button" variant="outline" size="sm" aria-label={msg("搜索合同")} onClick={() => setContractQuery(contractSearch.trim())}><Search size={15} /></Button></div>
          </div>
          {contractID && <button type="button" className="welfare-activity-selected-contract" onClick={() => setContractID("")}><span><FileText size={15} />{msg("合同 #{0}", contractID)}</span><X size={15} aria-hidden="true" /></button>}
          {contractList.isPending ? <p className="welfare-field-hint">{msg("正在读取合同")}</p> : contractList.isError ? <p role="alert">{msg("合同读取失败，请重试")}</p> : <div className="welfare-activity-contract-list">{contractItems.length ? contractItems.slice(0, 20).map((contract: Contract) => <button type="button" key={contract.id} className={`welfare-activity-contract-option${contract.id === contractID ? " is-selected" : ""}`} onClick={() => setContractID(contract.id)}><span><strong>{contractTitle(contract)}</strong><small>#{contract.id} · {contractAmount(contract.price || contract.reward)} ISK</small></span>{contract.id === contractID && <Check size={15} aria-hidden="true" />}</button>) : <p className="welfare-field-hint">{msg("没有匹配的合同")}</p>}</div>}
        </div>}
      </div>
      <div className="welfare-field"><span>{msg("活动截图（1–3 张，每张不超过 2 MB）")}</span><label className="welfare-activity-upload"><input aria-label={msg("选择截图")} type="file" accept="image/png,image/jpeg,image/webp" multiple onChange={(e) => setFiles(Array.from(e.target.files || []).slice(0, 3))} /><Camera size={18} aria-hidden="true" /><span>{msg("选择截图")}</span><small>{files.length}/3</small></label></div>
      {previews.length > 0 && <div className="welfare-activity-preview-grid" aria-label={msg("截图预览")}>{previews.map(({ file, url }, index) => <div className="welfare-activity-preview-item" key={`${file.name}:${file.lastModified}`}><a href={url} target="_blank" rel="noreferrer"><img src={url} alt={msg("活动截图 {0}", index + 1)} /></a><button type="button" aria-label={msg("移除截图 {0}", index + 1)} onClick={() => setFiles((current) => current.filter((_, item) => item !== index))}><X size={14} /></button></div>)}</div>}
      {files.length > 0 && !validFiles && <p role="alert">{msg("请上传 1–3 张 JPG、PNG 或 WebP 截图，每张不超过 2 MB")}</p>}
      {error && <p role="alert">{error}</p>}
    </form>
  </Modal>;
}
