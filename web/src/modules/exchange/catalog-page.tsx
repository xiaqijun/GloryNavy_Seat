import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { Library, Plus, Pencil, Search, Settings2, Coins } from "lucide-react";
import { msg } from "@/lib/i18n";
import { useSession } from "@/modules/identity";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { Modal } from "@/components/ui/dialog";
import { RewardSummary } from "@/components/reward-summary";
import {
  getCatalog,
  type CatalogEntry,
  type PhysicalContent,
} from "./catalog-api";
import { PhysicalFields } from "./physical-fields";
import { write, getContext } from "./api";
import { isClaimed, getRewardSettings } from "./rewards-api";
import { RateForm, RewardForm, SourceForm } from "./configuration-forms";
import "./exchange.css";
import "@/modules/welfare/welfare.css";

export default function CatalogPage() {
  const session = useSession();
  const cache = useQueryClient();
  const [editing, setEditing] = useState<CatalogEntry | "new" | null>(null);
  const [search, setSearch] = useState("");
  const [config, setConfig] = useState<{
    kind: "rate" | "source" | "reward";
    id?: string;
  } | null>(null);
  const settings = useQuery({
    queryKey: ["exchange", "reward-settings", session.data?.session?.user_id],
    queryFn: async ({ signal }) => {
      const [context, shop] = await Promise.all([
        getContext(signal),
        getRewardSettings(signal),
      ]);
      return { context, shop };
    },
    enabled: !!session.data?.session,
  });
  const q = useQuery({
    queryKey: ["catalog", "all"],
    queryFn: ({ signal }) => getCatalog(signal),
    enabled: !!session.data?.session,
  });
  const filtered = (q.data || []).filter((r) =>
    r.name.toLocaleLowerCase().includes(search.toLocaleLowerCase()),
  );
  const source = settings.data?.context.sources.find(
    (s) => s.id === config?.id,
  );
  const offer = settings.data?.shop.rewards.find((r) => r.id === config?.id);
  const configOpener = useRef<HTMLElement | null>(null);
  useEffect(() => {
    if (!config && configOpener.current) {
      configOpener.current.focus();
      configOpener.current = null;
    }
  }, [config]);
  const openConfig = (
    next: NonNullable<typeof config>,
    opener: HTMLElement,
  ) => {
    configOpener.current = opener;
    setConfig(next);
  };
  const closeConfig = () => {
    setConfig(null);
  };
  const savedConfig = () => {
    closeConfig();
    void cache.invalidateQueries({ queryKey: ["exchange"] });
    void cache.invalidateQueries({ queryKey: ["catalog"] });
  };
  return (
    <div className="exchange-page">
      <header className="exchange-heading">
        <span className="exchange-mark">
          <Library />
        </span>
        <h1>{msg("奖励库")}</h1>
      </header>
      {q.isError ? (
        <p role="alert">
          {q.error.message}
          <Button variant="outline" onClick={() => void q.refetch()}>
            {msg("重试")}
          </Button>
        </p>
      ) : !q.data ? (
        <p role="status">{msg("正在读取")}</p>
      ) : (
        <>
          <div className="exchange-toolbar">
            <label className="exchange-search">
              <Search size={18} aria-hidden="true" />
              <input
                aria-label={msg("搜索奖励")}
                placeholder={msg("搜索奖励")}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </label>
            <span className="exchange-spacer" />
            <Link to="/exchange">{msg("兑换中心")}</Link>
            <Button disabled={!!config} onClick={() => setEditing("new")}>
              <Plus />
              {msg("新增奖励")}
            </Button>
          </div>
          {settings.isError ? (
            <p role="alert">
              {settings.error.message}
              <Button variant="outline" onClick={() => void settings.refetch()}>
                {msg("重试")}
              </Button>
            </p>
          ) : (
            settings.data?.shop.admin && (
              <div className="exchange-toolbar">
                {settings.data.context.sources.map((s) => (
                  <Button
                    key={s.id}
                    variant="outline"
                    disabled={!!config}
                    onClick={(e) =>
                      openConfig({ kind: "source", id: s.id }, e.currentTarget)
                    }
                  >
                    <Coins />
                    {msg("{0}兑换比例", s.id === "pap" ? "PAP" : s.name)}
                  </Button>
                ))}
                <Button
                  variant="outline"
                  disabled={!!config}
                  onClick={(e) => openConfig({ kind: "rate" }, e.currentTarget)}
                >
                  <Settings2 />
                  {msg("果壳币价值")}
                </Button>
              </div>
            )
          )}
          {config && settings.data && session.data?.session && (
            <div
              className="catalog-configuration"
              ref={(node) => {
                if (node) node.scrollIntoView({ block: "nearest" });
              }}
            >
              {config.kind === "rate" && (
                <RateForm
                  csrf={session.data.session.csrf_token}
                  shop={settings.data.shop}
                  done={savedConfig}
                  close={closeConfig}
                />
              )}
              {config.kind === "source" && source && (
                <SourceForm
                  csrf={session.data.session.csrf_token}
                  source={source}
                  done={savedConfig}
                  close={closeConfig}
                />
              )}
              {config.kind === "reward" && offer && (
                <RewardForm
                  csrf={session.data.session.csrf_token}
                  reward={offer}
                  done={savedConfig}
                  close={closeConfig}
                />
              )}
            </div>
          )}
          {!filtered.length && (
            <Card className="exchange-card">
              {msg(search.trim() ? "暂无匹配奖励" : "暂无奖励")}
            </Card>
          )}
          <div className="catalog-grid">
            {filtered.map((r) => (
              <Card className="exchange-card catalog-card" key={r.id}>
                <div className="catalog-card-heading">
                  <div className="catalog-card-title">
                    <strong>{r.name}</strong>
                    {r.archived && <small>{msg("已停用")}</small>}
                  </div>
                  <IconAction
                    label={msg("编辑 {0}", r.name)}
                    disabled={!!config}
                    onClick={() => setEditing(r)}
                  >
                    <Pencil />
                  </IconAction>
                </div>
                <RewardSummary rewards={r.content} compact />
                {!r.archived &&
                  (() => {
                    const reward = settings.data?.shop.rewards.find(
                      (offer) => offer.id === r.id,
                    );
                    return (
                      <div className="exchange-toolbar catalog-offer">
                        <small className="exchange-note">
                          {reward
                            ? msg(reward.enabled ? "已上架" : "未上架")
                            : "—"}
                        </small>
                        <span className="exchange-spacer" />
                        <Button
                          variant="outline"
                          disabled={
                            !!config || !reward || !settings.data?.shop.admin
                          }
                          onClick={(e) =>
                            openConfig(
                              { kind: "reward", id: r.id },
                              e.currentTarget,
                            )
                          }
                        >
                          <Settings2 />
                          {msg("兑换设置")}
                        </Button>
                      </div>
                    );
                  })()}
              </Card>
            ))}
          </div>
        </>
      )}
      {editing && session.data?.session && (
        <Editor
          entry={editing === "new" ? undefined : editing}
          csrf={session.data.session.csrf_token}
          close={() => setEditing(null)}
          done={() => {
            setEditing(null);
            void cache.invalidateQueries({ queryKey: ["catalog"] });
            void cache.invalidateQueries({ queryKey: ["exchange"] });
          }}
        />
      )}
    </div>
  );
}
function Editor({
  entry,
  csrf,
  close,
  done,
}: {
  entry?: CatalogEntry;
  csrf: string;
  close: () => void;
  done: () => void;
}) {
  const [name, setName] = useState(entry?.name || "");
  const [content, setContent] = useState<PhysicalContent>(
    entry?.content || { fittings: [], items: [] },
  );
  const [archived, setArchived] = useState(entry?.archived || false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const retry = useRef({ body: "", key: crypto.randomUUID() });
  const errorRef = useRef<HTMLParagraphElement>(null);
  return (
    <Modal
      title={entry ? msg("编辑奖励") : msg("新增奖励")}
      close={close}
      busy={busy}
      footer={
        <>
          <Button variant="outline" disabled={busy} onClick={close}>
            {msg("取消")}
          </Button>
          <Button
            type="submit"
            form="catalog-edit"
            disabled={
              busy ||
              !name.trim() ||
              (!archived && !!content.isk_minor && content.isk_minor < 100) ||
              (!archived &&
                !content.fittings.length &&
                !content.items.length &&
                !content.isk_minor)
            }
          >
            {busy ? msg("正在保存") : msg("保存")}
          </Button>
        </>
      }
    >
      <form
        id="catalog-edit"
        className="welfare-form"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setError("");
          try {
            const body = {
              id: entry?.id || "0",
              version: entry?.version || "0",
              name,
              archived,
              content: {
                isk_minor: content.isk_minor || 0,
                fittings: content.fittings.map((f) => ({
                  fitting_id: f.fitting_id,
                  quantity: f.quantity,
                })),
                items: content.items.map((i) => ({
                  type_id: i.type_id,
                  quantity: i.quantity,
                })),
              },
            };
            const fingerprint = JSON.stringify(body);
            if (retry.current.body !== fingerprint)
              retry.current = { body: fingerprint, key: crypto.randomUUID() };
            await write(
              "/catalog",
              csrf,
              { ...body, request_key: retry.current.key },
              isClaimed,
            );
            done();
          } catch (e) {
            setError(e instanceof Error ? e.message : msg("保存失败"));
            requestAnimationFrame(() => errorRef.current?.focus());
          } finally {
            setBusy(false);
          }
        }}
      >
        <fieldset disabled={busy}>
          <label className="welfare-field wide">
            {msg("奖励名称")}
            <input
              value={name}
              required
              maxLength={100}
              onChange={(e) => setName(e.target.value)}
            />
          </label>
          {entry && (
            <>
              <label className="welfare-check">
                <input
                  type="checkbox"
                  checked={archived}
                  onChange={(e) => setArchived(e.target.checked)}
                />
                {msg("停用奖励")}
              </label>
              <small className="wide">
                {msg("修改后需重新设置兑换库存并上架，历史订单不变。")}
              </small>
            </>
          )}
          {!archived && (
            <PhysicalFields rewards={content} setRewards={setContent} />
          )}
        </fieldset>
        {error && (
          <p role="alert" tabIndex={-1} ref={errorRef}>
            {error}
          </p>
        )}
      </form>
    </Modal>
  );
}
