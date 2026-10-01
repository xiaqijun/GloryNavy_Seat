import { assetSlotLabel } from "@/lib/eve-terminology";
import { msg, getLocale } from "@/lib/i18n";
import { Modal, ConfirmDialog } from "@/components/ui/dialog";
import { useState, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, Navigate, useSearchParams } from "react-router-dom";

import {
  Upload,
  Download,
  Search,
  Orbit,
  BookOpen,
  GraduationCap,
  Pencil,
  Trash2,
  RefreshCw,
  ChevronRight,
  ArrowLeft,
  Check,
} from "lucide-react";
import { useSession } from "@/modules/identity";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { IconAction } from "@/components/ui/icon-action";
import { EveImage } from "@/components/eve-image";
import { PlanEditor } from "@/modules/skills/plan-editor";
import * as skills from "@/modules/skills/api";
import * as api from "./api";
import * as lib from "./library-api";
import { slotNames } from "./model";
import "./library.css";

export default function FittingsPage() {
  const s = useSession();
  const [p] = useSearchParams();
  if (s.isError) return <p role="alert">{s.error.message}</p>;
  if (!s.isSuccess) return <p role="status">{msg("正在读取")}</p>;
  if (!s.data.session) return <Navigate to="/login" replace />;
  return (
    <Library
      key={s.data.session.user_id + (p.get("member") ?? "")}
      member={p.get("member") ?? ""}
      csrf={s.data.session.csrf_token}
    />
  );
}
function Library({ member, csrf }: { member: string; csrf: string }) {
  const qc = useQueryClient();
  const [tab, setTab] = useState(member ? "personal" : "corporation");
  const [corpID, setCorpID] = useState("");
  const [charID, setCharID] = useState("");
  const [selected, setSelected] = useState("");
  const [search, setSearch] = useState("");
  const [group, setGroup] = useState("");
  const [importing, setImporting] = useState<lib.Entry | "new" | null>(null);
  const [showDetail, setShowDetail] = useState(false);
  const context = useQuery({
    queryKey: ["fittings", "library-context"],
    queryFn: ({ signal }) => lib.context(signal),
  });
  const personalContext = useQuery({
    queryKey: ["fittings", "context", member],
    queryFn: ({ signal }) => api.context(member, signal),
    enabled: tab === "personal",
  });
  const corps = context.data?.corporations ?? [];
  const corp = corps.find((c) => c.id === corpID) ?? corps[0];
  const characters = personalContext.data?.characters ?? [];
  const character = characters.find((c) => c.id === charID) ?? characters[0];
  const library = useQuery({
    queryKey: ["fittings", "library", corp?.id],
    queryFn: ({ signal }) => lib.list(corp!.id, signal),
    enabled: tab === "corporation" && !!corp,
  });
  const personal = useQuery({
    queryKey: ["fittings", "saved", character?.id],
    queryFn: ({ signal }) => api.snapshot(character!.id, "fittings", signal),
    enabled: tab === "personal" && !!character,
  });
  const shipIDs =
    tab === "corporation"
      ? []
      : [
          ...new Set(
            (personal.data?.fittings ?? []).map((f) => f.ship_type_id),
          ),
        ];
  const ships = useQuery({
    queryKey: ["fittings", "library-ship-names", shipIDs],
    queryFn: async ({ signal }) => {
      const rows: Awaited<ReturnType<typeof api.names>> = [];
      for (let i = 0; i < shipIDs.length; i += 512)
        rows.push(...(await api.names(shipIDs.slice(i, i + 512), signal)));
      return rows;
    },
    enabled: shipIDs.length > 0,
  });
  const shipNames = new Map((ships.data ?? []).map((n) => [n.id, n.name]));
  const choices =
    tab === "corporation"
      ? (library.data ?? []).map((f) => ({
          id: f.id,
          name: f.name,
          ship: f.fit.ship_type_id,
          shipName: f.ship_name,
          group: f.group,
          entry: f,
        }))
      : (personal.data?.fittings ?? []).map((f) => ({
          id: f.id,
          name: f.name,
          ship: f.ship_type_id,
          shipName:
            shipNames.get(f.ship_type_id) ?? msg("舰船 #{0}", f.ship_type_id),
          group: "",
          entry: undefined,
        }));
  const groups = [
    ...new Set(choices.map((f) => f.group).filter(Boolean)),
  ].sort();
  const visible = choices.filter(
    (f) =>
      (!group || f.group === group) &&
      `${f.name} ${f.shipName}`.toLowerCase().includes(search.toLowerCase()),
  );
  const chosen = visible.find((f) => f.id === selected) ?? visible[0];
  const saved = personal.data?.fittings.find((f) => f.id === chosen?.id);
  const loading =
    tab === "corporation" ? library.isFetching : personal.isFetching;
  const reset = () => {
    setSelected("");
    setGroup("");
    setShowDetail(false);
  };
  return (
    <div className="fitting-library">
      <header className="library-heading">
        <h1>{msg("舰船配置")}</h1>
        <div className="library-actions">
          <IconAction
            label={msg("刷新方案")}
            disabled={loading}
            onClick={() =>
              void qc.invalidateQueries({ queryKey: ["fittings"] })
            }
          >
            <RefreshCw size={18} />
          </IconAction>
          {context.data?.can_manage && tab === "corporation" && corp && (
            <Button onClick={() => setImporting("new")}>
              <Upload size={18} />
              {msg("导入配置")}{" "}
            </Button>
          )}
        </div>
      </header>
      <div className="library-toolbar">
        <nav aria-label={msg("配置方案库")}>
          <Button
            aria-pressed={tab === "corporation"}
            variant={tab === "corporation" ? "default" : "ghost"}
            onClick={() => {
              setTab("corporation");
              reset();
            }}
          >
            <BookOpen size={18} />
            {msg("军团方案")}{" "}
          </Button>
          <Button
            aria-pressed={tab === "personal"}
            variant={tab === "personal" ? "default" : "ghost"}
            onClick={() => {
              setTab("personal");
              reset();
            }}
          >
            <Orbit size={18} />
            {msg("个人方案")}{" "}
          </Button>
        </nav>
        {tab === "corporation" ? (
          <Select
            label={msg("军团")}
            value={corp?.id ?? ""}
            onValueChange={(v) => {
              setCorpID(v);
              reset();
            }}
            options={corps.map((c) => ({ value: c.id, label: c.name }))}
          />
        ) : (
          <Select
            label={msg("查看角色")}
            value={character?.id ?? ""}
            onValueChange={(v) => {
              setCharID(v);
              reset();
            }}
            options={characters.map((c) => ({ value: c.id, label: c.name }))}
          />
        )}
      </div>
      {member && tab === "personal" && (
        <p className="muted">{msg("成员个人方案 · 只读")}</p>
      )}
      {[
        context.error,
        library.error,
        personal.error,
        personalContext.error,
        ships.error,
      ]
        .filter(Boolean)
        .map((e, i) => (
          <p role="alert" key={i}>
            {e?.message}
          </p>
        ))}
      {context.isPending && <p role="status">{msg("正在读取")}</p>}
      <div className={`library-layout ${showDetail ? "show-detail" : ""}`}>
        <Card className="library-browser">
          <div className="library-search">
            <Search size={18} />
            <input
              aria-label={msg("搜索方案或船型")}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={msg("搜索方案、船型")}
            />
          </div>
          {groups.length > 0 && (
            <Select
              label={msg("舰船分类")}
              value={group}
              onValueChange={setGroup}
              options={[
                { value: "", label: msg("全部船型") },
                ...groups.map((g) => ({ value: g, label: g })),
              ]}
            />
          )}
          <div className="library-count">
            <span>
              {visible.length} {msg("个方案")}
            </span>
            {tab === "personal" && personal.data?.observed_at && (
              <span>
                {new Date(personal.data.observed_at).toLocaleDateString(
                  getLocale(),
                )}
              </span>
            )}
          </div>
          {!chosen ? (
            <p className="library-empty">
              {loading
                ? msg("正在读取方案")
                : tab === "personal" && personal.data?.status === "pending"
                  ? msg("个人方案待同步")
                  : search || group
                    ? msg("没有匹配方案")
                    : msg("暂无配置方案")}
            </p>
          ) : (
            <div className="library-choices">
              {visible.map((f) => (
                <button
                  className={chosen.id === f.id ? "selected" : ""}
                  key={f.id}
                  aria-pressed={chosen.id === f.id}
                  onClick={() => {
                    setSelected(f.id);
                    setShowDetail(true);
                  }}
                >
                  <EveImage kind="type" id={f.ship} />
                  <span>
                    <strong>{f.name}</strong>
                    <small>{f.shipName}</small>
                  </span>
                  <ChevronRight size={16} />
                </button>
              ))}
            </div>
          )}
        </Card>
        {chosen ? (
          <Card className="library-detail">
            <Button
              className="library-back"
              variant="ghost"
              onClick={() => setShowDetail(false)}
            >
              <ArrowLeft size={16} />
              {msg("返回方案列表")}{" "}
            </Button>
            <Detail
              key={`${tab}:${chosen.id}:${chosen.entry?.version ?? character?.id}`}
              csrf={csrf}
              entry={chosen.entry}
              saved={tab === "personal" ? saved : undefined}
              shipName={chosen.shipName}
              context={context.data}
              canSkills={!!corp?.can_create_skills}
              onEdit={() => chosen.entry && setImporting(chosen.entry)}
              onDelete={() => {
                setSelected("");
                void qc.invalidateQueries({
                  queryKey: ["fittings", "library"],
                });
              }}
            />
          </Card>
        ) : (
          <Card className="library-detail library-placeholder">
            <Orbit size={40} />
            <h2>{msg("选择配置方案")}</h2>
          </Card>
        )}
      </div>
      {importing && corp && (
        <ImportDialog
          corp={corp.id}
          csrf={csrf}
          entry={importing === "new" ? null : importing}
          close={() => setImporting(null)}
          saved={(f) => {
            setImporting(null);
            setSelected(f.id);
            setShowDetail(true);
            void qc.invalidateQueries({ queryKey: ["fittings", "library"] });
          }}
        />
      )}
    </div>
  );
}
function Detail({
  csrf,
  entry,
  saved,
  shipName,
  context,
  canSkills,
  onEdit,
  onDelete,
}: {
  csrf: string;
  entry?: lib.Entry;
  saved?: api.Snapshot["fittings"][number];
  shipName: string;
  context?: lib.LibraryContext;
  canSkills: boolean;
  onEdit: () => void;
  onDelete: () => void;
}) {
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [deleting, setDeleting] = useState<lib.Entry | null>(null);
  const [deleteError, setDeleteError] = useState("");
  const [saving, setSaving] = useState(false);
  const [seed, setSeed] = useState<{
    name: string;
    corp: string;
    requirements: skills.Requirement[];
    types: skills.SkillType[];
  } | null>(null);
  const [linked, setLinked] = useState<skills.Plan | null>(null);
  const f = entry?.fit;
  const items = f
    ? f.items.map((i) => ({
        id: i.type_id,
        slot: slotNames[i.slot],
        quantity: i.quantity,
        charge: i.charge_id,
      }))
    : (saved?.items.map((i) => ({
        id: i.type_id,
        slot: assetSlotLabel(i.flag),
        quantity: i.quantity,
        charge: undefined,
      })) ?? []);
  const ids = [
    ...new Set(items.flatMap((i) => (i.charge ? [i.id, i.charge] : [i.id]))),
  ];
  const names = useQuery({
    queryKey: ["fittings", "library-item-names", ids],
    queryFn: async ({ signal }) => {
      const out: api.Named[] = [];
      for (let i = 0; i < ids.length; i += 512)
        out.push(...(await api.names(ids.slice(i, i + 512), signal)));
      return out;
    },
    enabled: ids.length > 0,
  });
  const named = new Map((names.data ?? []).map((n) => [n.id, n.name]));
  const groups = [...new Set(items.map((i) => i.slot))];
  const createSkills = async () => {
    if (!entry) return;
    setBusy(true);
    setMessage("");
    try {
      const [req, types] = await Promise.all([
        lib.requirements(entry.id),
        skills.catalog(),
      ]);
      setSeed({
        name: req.name,
        corp: req.corporation_id,
        requirements: req.requirements,
        types: types.items,
      });
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const remove = async () => {
    if (!deleting || busy) return;
    setBusy(true);
    setDeleteError("");
    try {
      await lib.publish(
        deleting.id,
        {
          corporation_id: deleting.corporation_id,
          version: deleting.version,
          request_key: "",
          description: deleting.description,
          eft: deleting.eft,
        },
        csrf,
        true,
      );
      setDeleting(null);
      onDelete();
    } catch (e) {
      setDeleteError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const download = () => {
    if (!entry) return;
    const url = URL.createObjectURL(
      new Blob([entry.eft], { type: "text/plain;charset=utf-8" }),
    );
    const a = document.createElement("a");
    a.href = url;
    a.download = entry.name.replace(/[<>:"/\\|?*]/g, "_") + ".txt";
    a.click();
    URL.revokeObjectURL(url);
  };
  return (
    <>
      {deleting && (
        <ConfirmDialog
          title={msg("删除配置")}
          description={msg("删除“{0}”？此操作无法撤销。", deleting.name)}
          confirmLabel={msg("删除配置")}
          busy={busy}
          error={deleteError}
          onConfirm={() => void remove()}
          onClose={() => setDeleting(null)}
        />
      )}
      <header className="library-ship">
        <EveImage
          kind="type"
          id={f?.ship_type_id ?? saved?.ship_type_id ?? ""}
          className="library-ship-image"
        />
        <div>
          <span className="library-ship-type">{shipName}</span>
          <h2>{entry?.name ?? saved?.name}</h2>
          {entry?.group && <span className="library-label">{entry.group}</span>}
        </div>
      </header>
      {(entry?.description || saved?.description) && (
        <p className="library-description">
          {entry?.description ?? saved?.description}
        </p>
      )}
      {entry && (
        <div className="library-detail-actions">
          <Button onClick={() => setSaving(true)}>
            <Download size={18} />
            {msg("保存到个人配置")}{" "}
          </Button>
          {canSkills && (
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => void createSkills()}
            >
              <GraduationCap size={18} />
              {linked ? msg("编辑技能方案") : msg("创建技能方案")}
            </Button>
          )}
          <IconAction label={msg("导出 EFT")} onClick={download}>
            <Download size={18} />
          </IconAction>
          {context?.can_manage && (
            <>
              <IconAction
                label={msg("替换配置")}
                disabled={busy}
                onClick={onEdit}
              >
                <Pencil size={18} />
              </IconAction>
              <IconAction
                label={msg("删除配置")}
                disabled={busy}
                onClick={() => {
                  if (entry) {
                    setDeleteError("");
                    setDeleting(entry);
                  }
                }}
              >
                <Trash2 size={18} />
              </IconAction>
            </>
          )}
          {linked && (
            <Link
              to={`/skills?${new URLSearchParams({ plan: linked.id, corporation: linked.corporation_id })}`}
            >
              {msg("查看技能方案")}{" "}
            </Link>
          )}
        </div>
      )}
      {message && <p role="alert">{message}</p>}
      {names.error && <p role="alert">{names.error.message}</p>}
      <div className="library-fitting-groups">
        {groups.map((g) => (
          <section key={g}>
            <h3>
              {g}
              <span>{items.filter((i) => i.slot === g).length}</span>
            </h3>
            {items
              .filter((i) => i.slot === g)
              .map((i, n) => (
                <div className="library-item" key={`${i.id}:${n}`}>
                  <EveImage kind="type" id={i.id} />
                  <div>
                    <span>{named.get(i.id) ?? msg("物品 #{0}", i.id)}</span>
                    {i.charge && (
                      <small>
                        {named.get(i.charge) ?? msg("弹药 #{0}", i.charge)}
                      </small>
                    )}
                  </div>
                  <b>×{i.quantity.toLocaleString(getLocale())}</b>
                </div>
              ))}
          </section>
        ))}
      </div>
      {saving && entry && (
        <GameSaveDialog
          entry={entry}
          characters={context?.characters ?? []}
          csrf={csrf}
          close={() => setSaving(false)}
        />
      )}
      {seed && (
        <PlanEditor
          csrf={csrf}
          corp={seed.corp}
          plan={linked}
          initialName={seed.name}
          initialRequirements={seed.requirements}
          types={seed.types}
          close={() => setSeed(null)}
          saved={(p) => {
            setLinked(p);
            setSeed(null);
          }}
        />
      )}
    </>
  );
}
function ImportDialog({
  corp,
  csrf,
  entry,
  close,
  saved,
}: {
  corp: string;
  csrf: string;
  entry: lib.Entry | null;
  close: () => void;
  saved: (e: lib.Entry) => void;
}) {
  const [eft, setEft] = useState(entry?.eft ?? "");
  const [description, setDescription] = useState(entry?.description ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const key = useRef(crypto.randomUUID());
  const submit = async () => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      saved(
        await lib.publish(
          entry?.id ?? "",
          {
            corporation_id: corp,
            eft,
            description,
            version: entry?.version ?? "0",
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
      className="library-dialog"
      title={entry ? msg("替换配置") : msg("导入配置")}
      close={close}
      busy={busy}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <label>
          {msg("装配方案")}{" "}
          <textarea
            autoFocus
            required
            rows={11}
            value={eft}
            disabled={busy}
            onChange={(e) => setEft(e.target.value)}
            placeholder={msg("粘贴游戏中复制的 EFT 配置\n[舰船名称, 方案名称]")}
          />
        </label>
        <label>
          {msg("说明")}{" "}
          <input
            maxLength={500}
            value={description}
            disabled={busy}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={msg("可选")}
          />
        </label>
        {error && <p role="alert">{error}</p>}
        <Button type="submit" disabled={busy || !eft.trim()}>
          <Upload size={18} />
          {busy ? msg("正在导入") : entry ? msg("保存替换") : msg("导入军团库")}
        </Button>
      </form>
    </Modal>
  );
}
function GameSaveDialog({
  entry,
  characters,
  csrf,
  close,
}: {
  entry: lib.Entry;
  characters: lib.LibraryContext["characters"];
  csrf: string;
  close: () => void;
}) {
  const [selected, setSelected] = useState(characters[0]?.id ?? "");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<lib.GameSave | null>(null);
  const [error, setError] = useState("");
  const key = useRef(crypto.randomUUID());
  const character = characters.find((c) => c.id === selected);
  const qc = useQueryClient();
  const submit = async () => {
    if (!character || busy) return;
    setBusy(true);
    setError("");
    try {
      const r = await lib.save(
        entry.id,
        entry.version,
        character.id,
        key.current,
        csrf,
      );
      setResult(r);
      if (r.state === "saved")
        void qc.invalidateQueries({ queryKey: ["fittings", "saved"] });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  const reasons: Record<string, string> = {
    missing_scope: msg("需要补充配装写入授权"),
    access_denied: msg("游戏拒绝保存，请检查方案数量上限或授权"),
    upstream_unavailable: msg("游戏未接受该方案，请检查名称和物品"),
    rate_limited: msg("请求受限，请稍后重试"),
    identity_changed: msg("角色归属或授权已变化"),
    credential_unavailable: msg("角色授权不可用"),
    reauthorize: msg("需要重新授权该角色"),
    service_unavailable: msg("保存服务暂不可用"),
    confirmation_required: msg("结果尚未确认，请先在游戏中核对，避免重复保存"),
  };
  return (
    <Modal
      className="library-dialog"
      title={msg("保存到游戏")}
      close={close}
      busy={busy}
    >
      <p>{entry.name}</p>
      <Select
        label={msg("接收角色")}
        value={selected}
        disabled={busy || (!!result && result.state !== "failed")}
        onValueChange={(v) => {
          setSelected(v);
          setResult(null);
          setError("");
          key.current = crypto.randomUUID();
        }}
        options={characters.map((c) => ({ value: c.id, label: c.name }))}
      />
      {character && !character.can_save && (
        <p>
          {msg("需要补充配装写入授权。")}
          <Link to="/account">{msg("前往重新授权")}</Link>
        </p>
      )}
      {error && <p role="alert">{error}</p>}
      {result && (
        <p role="status">
          {result.state === "saved"
            ? msg(
                "已保存到 {0} 的游戏个人方案，本站列表将在下次同步后更新",
                character?.name,
              )
            : result.state === "sending"
              ? msg("正在确认保存结果")
              : (reasons[result.reason] ?? msg("保存未完成，请检查授权后重试"))}
        </p>
      )}
      <Button
        disabled={
          busy ||
          !character?.can_save ||
          (!!result && result.state !== "failed") ||
          !!error
        }
        onClick={() => void submit()}
      >
        {result?.state === "saved" ? (
          <Check size={18} />
        ) : (
          <Download size={18} />
        )}{" "}
        {busy ? msg("正在保存") : msg("保存到游戏")}
      </Button>
    </Modal>
  );
}
