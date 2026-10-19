import { useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { RefreshCw, Search } from "lucide-react";
import { SearchInput } from "@/components/search-input";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { Select } from "@/components/ui/select";
import { msg } from "@/lib/i18n";
import { normalizeSearch } from "@/lib/use-search-draft";
import {
  amount,
  contractDate,
  entityName,
  title,
} from "@/modules/eve/contracts-api";
import { capitalContracts } from "./capital-contracts";
import { capitalMoney } from "./capital-api";
import type { Character } from "./api";

export function CapitalContractPicker({
  characters,
  character,
  onCharacterChange,
  value,
  onChange,
}: {
  characters: Character[];
  character: string;
  onCharacterChange: (id: string) => void;
  value: string;
  onChange: (id: string) => void;
}) {
  const [draft, setDraft] = useState("");
  const [search, setSearch] = useState("");
  const owner = characters.find((c) => c.id === character);
  const list = useInfiniteQuery({
    queryKey: [
      "welfare",
      "capital-contracts",
      owner?.account_id,
      character,
      search,
    ],
    initialPageParam: "",
    enabled: !!owner,
    queryFn: ({ pageParam, signal }) =>
      capitalContracts(
        { kind: "character", id: character, name: owner!.name },
        search,
        pageParam,
        signal,
      ),
    getNextPageParam: (page) => page.next_cursor || undefined,
    retry: false,
  });
  const candidates = list.data?.pages.flatMap((page) => page.items) ?? [];
  const applySearch = () => {
    onChange("");
    setSearch(normalizeSearch(draft));
  };
  return (
    <section
      className="wide welfare-capital-picker"
      aria-label={msg("购舰合同")}
    >
      {characters.length > 1 ? (
        <Select
          label={msg("购舰角色")}
          value={character}
          onValueChange={(id) => {
            onChange("");
            onCharacterChange(id);
          }}
          options={characters.map((c) => ({ value: c.id, label: c.name }))}
        />
      ) : owner ? (
        <strong>{owner.name}</strong>
      ) : (
        <p role="status">{msg("请先绑定本军团角色")}</p>
      )}
      {owner && (
        <>
          <div className="welfare-capital-search">
            <SearchInput
              aria-label={msg("搜索购舰合同")}
              placeholder={msg("合同 ID、标题、物品或交易方")}
              maxLength={100}
              value={draft}
              onChange={(e) => {
                setDraft(e.target.value);
                onChange("");
              }}
              onClear={() => {
                setDraft("");
                setSearch("");
                onChange("");
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  applySearch();
                }
              }}
            />
            <IconAction type="button" label={msg("搜索")} onClick={applySearch}>
              <Search size={18} aria-hidden="true" />
            </IconAction>
            <IconAction
              type="button"
              label={msg("刷新")}
              disabled={list.isFetching}
              aria-busy={list.isFetching}
              onClick={() => {
                onChange("");
                void list.refetch();
              }}
            >
              <RefreshCw size={18} aria-hidden="true" />
            </IconAction>
          </div>
          {list.isPending && <p role="status">{msg("正在读取")}</p>}
          {list.isError && (
            <div className="welfare-capital-error" role="alert">
              {list.error.message}
              <Button
                type="button"
                variant="outline"
                onClick={() => void list.refetch()}
              >
                {msg("重试")}
              </Button>
            </div>
          )}
          {list.isSuccess && !candidates.length && (
            <p role="status">{msg("当前没有可选合同")}</p>
          )}
          {candidates.length > 0 && <p role="status">{msg("已加载 {0} 份合同", candidates.length)}</p>}
          {candidates.length > 0 && (
            <div
              className="welfare-capital-contracts"
              role="radiogroup"
              aria-label={msg("选择购舰合同")}
            >
              {candidates.map((c) => (
                <label
                  className={`welfare-capital-contract${value === c.id ? " is-selected" : ""}`}
                  key={c.id}
                >
                  <input
                    type="radio"
                    name="capital-contract"
                    value={c.id}
                    checked={value === c.id}
                    disabled={
                      list.isFetching ||
                      list.isError ||
                      normalizeSearch(draft) !== search
                    }
                    onChange={() => onChange(c.id)}
                    aria-label={`${title(c)} #${c.id}`}
                  />
                  <span>
                    <strong>{title(c)}</strong>
                    {c.title.trim() &&
                      c.summary?.trim() &&
                      c.title.trim() !== c.summary.trim() && (
                        <span>{c.summary}</span>
                      )}
                    <span>
                      #{c.id} · {entityName(c.issuer)} ·{" "}
                      {contractDate(c.date_completed)}
                    </span>
                  </span>
                  <b title={`${amount(c.price)} ISK`}>
                    {capitalMoney(Number(c.price) * 100)} ISK
                  </b>
                </label>
              ))}
            </div>
          )}
          {list.hasNextPage && (
            <Button
              type="button"
              variant="outline"
              disabled={list.isFetching || normalizeSearch(draft) !== search}
              onClick={() => void list.fetchNextPage()}
            >
              {list.isFetchingNextPage ? msg("正在读取") : msg("加载更多")}
            </Button>
          )}
        </>
      )}
    </section>
  );
}
