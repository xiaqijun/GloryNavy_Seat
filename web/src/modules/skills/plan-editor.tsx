import { msg } from "@/lib/i18n";
import { useId, useRef, useState } from "react";
import { Modal } from "@/components/ui/dialog";
import { Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { Select } from "@/components/ui/select";
import * as api from "./api";
import "./skills.css";
export function PlanEditor({
  csrf,
  corp,
  plan,
  types,
  close,
  saved,
  initialName = "",
  initialRequirements = [],
}: {
  initialName?: string;
  initialRequirements?: api.Requirement[];
  csrf: string;
  corp: string;
  plan: api.Plan | null;
  types: api.SkillType[];
  close: () => void;
  saved: (p: api.Plan) => void;
}) {
  const [name, setName] = useState(plan?.name ?? initialName);
  const [requirements, setRequirements] = useState<api.Requirement[]>(
    plan?.requirements ?? initialRequirements,
  );
  const [search, setSearch] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const key = useRef(crypto.randomUUID());
  const formID = useId();
  const lookup = new Map(types.map((t) => [t.id, t.name]));
  const results = search.trim()
    ? types
        .filter(
          (t) =>
            !requirements.some((r) => r.skill_id === t.id) &&
            (t.name.toLowerCase().includes(search.toLowerCase()) ||
              t.english.toLowerCase().includes(search.toLowerCase()) ||
              t.id === search),
        )
        .slice(0, 20)
    : [];
  const submit = async () => {
    if (busy) return;
    setError("");
    setBusy(true);
    try {
      saved(
        await api.save(
          plan?.id ?? "",
          {
            corporation_id: corp,
            name,
            requirements,
            version: plan?.version ?? "0",
            request_key: key.current,
          },
          csrf,
        ),
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal
      title={plan ? msg("编辑技能要求") : msg("新建技能要求")}
      close={close}
      busy={busy}
      className="skills-dialog"
      footer={
        <>
          <span className="muted">
            {requirements.length} {msg("项技能")}
          </span>
          <Button
            form={formID}
            type="submit"
            disabled={busy || !name.trim() || !requirements.length}
          >
            {busy ? msg("正在保存") : msg("保存要求")}
          </Button>
        </>
      }
    >
      <form
        id={formID}
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <label className="skill-field">
          {msg("方案名称")}{" "}
          <input
            autoFocus
            required
            maxLength={80}
            value={name}
            disabled={busy}
            onChange={(e) => setName(e.target.value)}
            placeholder={msg("例如：集结基础技能")}
          />
        </label>
        <label className="skill-field">
          {msg("添加技能")}{" "}
          <input
            value={search}
            disabled={busy}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={msg("搜索中文、英文或技能 ID")}
          />
        </label>
        {search && (
          <div className="skill-pick">
            {results.length === 0 ? (
              <p>{msg("没有匹配的技能")}</p>
            ) : (
              results.map((t) => (
                <button
                  key={t.id}
                  type="button"
                  disabled={busy || requirements.length >= 200}
                  onClick={() => {
                    setRequirements([
                      ...requirements,
                      { skill_id: t.id, level: 5 },
                    ]);
                    setSearch("");
                  }}
                >
                  <span>
                    {t.name}
                    <small>{t.group}</small>
                  </span>
                  <Plus size={18} />
                </button>
              ))
            )}
          </div>
        )}
        <div className="skill-editor-rows">
          {requirements.map((r) => (
            <div className="skills-row" key={r.skill_id}>
              <span>
                {lookup.get(r.skill_id) ?? msg("技能 #{0}", r.skill_id)}
              </span>
              <Select
                label={msg(
                  "{0} 要求等级",
                  lookup.get(r.skill_id) ?? r.skill_id,
                )}
                disabled={busy}
                value={String(r.level)}
                onValueChange={(v) =>
                  setRequirements(
                    requirements.map((x) =>
                      x.skill_id === r.skill_id
                        ? { ...x, level: Number(v) }
                        : x,
                    ),
                  )
                }
                options={[1, 2, 3, 4, 5].map((n) => ({
                  value: String(n),
                  label: msg("{0} 级", n),
                }))}
              />
              <IconAction
                type="button"
                label={msg("移除{0}", lookup.get(r.skill_id) ?? r.skill_id)}
                disabled={busy}
                onClick={() =>
                  setRequirements(
                    requirements.filter((x) => x.skill_id !== r.skill_id),
                  )
                }
              >
                <X size={16} />
              </IconAction>
            </div>
          ))}
        </div>
        {error && <p role="alert">{error}</p>}
      </form>
    </Modal>
  );
}
