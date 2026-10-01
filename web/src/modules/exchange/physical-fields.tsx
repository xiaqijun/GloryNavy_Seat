import { useState, useDeferredValue } from "react";
import { useQuery } from "@tanstack/react-query";
import { Plus, Trash2, Package } from "lucide-react";
import { msg } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/select";
import { IconAction } from "@/components/ui/icon-action";
import { EveImage } from "@/components/eve-image";
import { context, list } from "@/modules/fittings/library-api";
import { getTypes } from "./rewards-api";
import type { PhysicalContent } from "./catalog-api";
export function PhysicalFields({
  rewards,
  setRewards,
}: {
  rewards: PhysicalContent;
  setRewards: (v: PhysicalContent) => void;
}) {
  const [corp, setCorp] = useState("");
  const [extraFit, setExtraFit] = useState("");
  const [search, setSearch] = useState("");
  const query = useDeferredValue(search.trim());
  const contextQuery = useQuery({
    queryKey: ["catalog", "fit-context"],
    queryFn: ({ signal }) => context(signal),
  });
  const selectedCorp = corp || contextQuery.data?.corporations[0]?.id || "";
  const fits = useQuery({
    queryKey: ["catalog", "fits", selectedCorp],
    queryFn: ({ signal }) => list(selectedCorp, signal),
    enabled: !!selectedCorp,
  });
  const items = useQuery({
    queryKey: ["catalog", "types", query],
    queryFn: async ({ signal }) => ({ items: await getTypes(query, signal) }),
    enabled: query.length >= 2,
  });
  const availableFits = (fits.data || [])
    .filter((f) => !rewards.fittings.some((r) => r.fitting_id === f.id))
    .map((f) => ({ value: f.id, label: f.name }));
  const canAddFit = availableFits.some((f) => f.value === extraFit);
  return (
    <>
      <label className="welfare-field wide">
        {msg("配装来源军团")}
        <Select
          label={msg("配装来源军团")}
          value={selectedCorp}
          onValueChange={(v) => {
            setCorp(v);
            setExtraFit("");
          }}
          options={(contextQuery.data?.corporations || []).map((c) => ({
            value: c.id,
            label: c.name,
          }))}
        />
      </label>
      {contextQuery.isError && <p role="alert">{contextQuery.error.message}</p>}
      {fits.isError && <p role="alert">{fits.error.message}</p>}
      <section
        className="growth-reward-editor wide"
        aria-label={msg("配装舰船奖励")}
      >
        <strong>{msg("配装舰船奖励")}</strong>
        {rewards.fittings.map((f, index) => (
          <div className="growth-reward-row" key={f.fitting_id}>
            <EveImage kind="type" id={f.ship_type_id || "0"} />
            <span>{f.name || `#${f.fitting_id}`}</span>
            <input
              aria-label={msg("{0}数量", f.name || f.fitting_id)}
              type="number"
              min={1}
              max={100}
              step={1}
              required
              value={f.quantity}
              onChange={(e) =>
                setRewards({
                  ...rewards,
                  fittings: rewards.fittings.map((v, n) =>
                    n === index
                      ? { ...v, quantity: Number(e.target.value) }
                      : v,
                  ),
                })
              }
            />
            <IconAction
              label={msg("移除{0}", f.name || f.fitting_id)}
              onClick={() =>
                setRewards({
                  ...rewards,
                  fittings: rewards.fittings.filter((_, n) => n !== index),
                })
              }
            >
              <Trash2 size={16} />
            </IconAction>
          </div>
        ))}
        {availableFits.length > 0 && rewards.fittings.length < 10 ? (
          <div className="growth-add-row">
            <Select
              label={msg("添加配装舰船")}
              placeholder={msg("选择其他配装")}
              value={extraFit}
              onValueChange={setExtraFit}
              options={availableFits}
            />
            <Button
              type="button"
              variant="outline"
              disabled={!canAddFit}
              onClick={() => {
                if (!canAddFit) return;
                const f = fits.data?.find((v) => v.id === extraFit);
                if (f)
                  setRewards({
                    ...rewards,
                    fittings: [
                      ...rewards.fittings,
                      {
                        fitting_id: f.id,
                        quantity: 1,
                        name: f.name,
                        ship_type_id: f.fit.ship_type_id,
                      },
                    ],
                  });
                setExtraFit("");
              }}
            >
              <Plus size={16} />
              {msg("添加")}
            </Button>
          </div>
        ) : fits.isSuccess && rewards.fittings.length > 0 ? (
          <small>
            {rewards.fittings.length >= 10
              ? msg("最多添加 10 种配装")
              : msg("军团配装已全部添加")}
          </small>
        ) : null}
      </section>
      <section
        className="growth-reward-editor wide"
        aria-label={msg("物品 / PLEX")}
      >
        <div className="exchange-toolbar">
          <strong>{msg("物品 / PLEX")}</strong>
          <Button
            type="button"
            variant="ghost"
            onClick={() => setSearch("PLEX")}
          >
            PLEX
          </Button>
        </div>
        {rewards.items.map((i, index) => (
          <div className="growth-reward-row" key={i.type_id}>
            <EveImage kind="type" id={i.type_id} />
            <span>{i.name || `#${i.type_id}`}</span>
            <input
              aria-label={msg("{0}数量", i.name || i.type_id)}
              type="number"
              min={1}
              max={1000000}
              step={1}
              required
              value={i.quantity}
              onChange={(e) =>
                setRewards({
                  ...rewards,
                  items: rewards.items.map((v, n) =>
                    n === index
                      ? { ...v, quantity: Number(e.target.value) }
                      : v,
                  ),
                })
              }
            />
            <IconAction
              label={msg("移除{0}", i.name || i.type_id)}
              onClick={() =>
                setRewards({
                  ...rewards,
                  items: rewards.items.filter((_, n) => n !== index),
                })
              }
            >
              <Trash2 size={16} />
            </IconAction>
          </div>
        ))}
        <label className="welfare-field">
          {msg("搜索奖励物品")}
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            maxLength={80}
          />
        </label>
        {items.isError && <p role="alert">{items.error.message}</p>}
        {query.length >= 2 && (
          <div className="growth-item-options">
            {items.data?.items
              .filter((i) => !rewards.items.some((v) => v.type_id === i.id))
              .map((i) => (
                <button
                  type="button"
                  disabled={rewards.items.length >= 30}
                  key={i.id}
                  onClick={() => {
                    setRewards({
                      ...rewards,
                      items: [
                        ...rewards.items,
                        { type_id: i.id, name: i.name, quantity: 1 },
                      ],
                    });
                    setSearch("");
                  }}
                >
                  <Package size={16} />
                  {i.name}
                  <Plus size={14} />
                </button>
              ))}
          </div>
        )}
      </section>
      <label className="welfare-field wide">
        {msg("ISK 奖励")}
        <input
          type="number"
          min={0}
          max={1e12}
          step="0.01"
          required
          value={(rewards.isk_minor || 0) / 100}
          onChange={(e) =>
            setRewards({
              ...rewards,
              isk_minor: Number.isFinite(e.target.valueAsNumber)
                ? Math.round(e.target.valueAsNumber * 100)
                : 0,
            })
          }
        />
        {(rewards.isk_minor || 0) > 0 && (rewards.isk_minor || 0) < 100 && (
          <small role="alert">{msg("ISK 奖励至少 1 ISK")}</small>
        )}
      </label>
    </>
  );
}
