import { useState } from "react";
import { useInfiniteQuery, useMutation, useQuery } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { Modal } from "@/components/ui/dialog";
import { Select } from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { msg } from "@/lib/i18n";
import {
  getContractOwners,
  getContracts,
  title,
  amount,
  contractTypes,
  contractDate,
  type Contract,
  type Owner,
} from "@/modules/eve/contracts-api";
import { estimateContract, type Appraisal, type ContractSide } from "./api";

export function ContractPicker({
  user,
  csrf,
  close,
  done,
}: {
  user: string;
  csrf: string;
  close: () => void;
  done: (result: Appraisal, source: string) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [ownerKey, setOwnerKey] = useState("");
  const owners = useQuery({
    queryKey: ["market", "contract-owners", user],
    queryFn: ({ signal }) => getContractOwners(signal),
  });
  const owner =
    owners.data?.owners.find((o) => `${o.kind}:${o.id}` === ownerKey) ??
    owners.data?.owners[0];
  return (
    <Modal title={msg("从合同估价")} close={close} busy={busy}>
      <div className="market-contract-picker">
        {owners.isPending && <p role="status">{msg("正在读取")}</p>}
        {owners.isError && (
          <p role="alert">
            {owners.error.message}{" "}
            <Button variant="outline" onClick={() => void owners.refetch()}>
              {msg("重试")}
            </Button>
          </p>
        )}
        {owners.data && !owners.data.owners.length && (
          <p role="status">{msg("暂无可查看的合同来源")}</p>
        )}
        {owner && (
          <>
            <Select
              disabled={busy}
              label={msg("合同来源")}
              value={`${owner.kind}:${owner.id}`}
              onValueChange={setOwnerKey}
              options={owners.data!.owners.map((o) => ({
                value: `${o.kind}:${o.id}`,
                label: o.name,
                group:
                  o.kind === "character" ? msg("个人合同") : msg("军团合同"),
              }))}
            />
            <ContractList
              key={`${owner.kind}:${owner.id}`}
              user={user}
              owner={owner}
              csrf={csrf}
              done={done}
              setBusy={setBusy}
            />
          </>
        )}
      </div>
    </Modal>
  );
}

function ContractList({
  user,
  owner,
  csrf,
  done,
  setBusy,
}: {
  setBusy: (busy: boolean) => void;
  user: string;
  owner: Owner;
  csrf: string;
  done: (result: Appraisal, source: string) => void;
}) {
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState("");
  const [selected, setSelected] = useState<Contract | null>(null);
  const [side, setSide] = useState<ContractSide>("included");
  const list = useInfiniteQuery({
    queryKey: ["market", "contracts", user, owner.kind, owner.id, filter],
    initialPageParam: "",
    queryFn: async ({ pageParam, signal }) => {
      const page = await getContracts(
        owner,
        new URLSearchParams({
          status: "outstanding",
          q: filter,
          before: pageParam,
        }),
        signal,
      );
      const now = Date.now();
      return {
        ...page,
        items: page.items.filter(
          (c) =>
            c.status === "outstanding" &&
            ["item_exchange", "auction"].includes(c.type) &&
            c.acceptor.id === "0" &&
            !c.date_accepted &&
            Date.parse(c.date_expired) > now,
        ),
      };
    },
    getNextPageParam: (page) => page.next_cursor || undefined,
  });
  const candidates = list.data?.pages.flatMap((p) => p.items) ?? [];
  const estimate = useMutation({
    onMutate: () => setBusy(true),
    onSettled: () => setBusy(false),
    mutationFn: (input: { contract: Contract; side: ContractSide }) =>
      estimateContract(csrf, owner, input.contract.id, input.side),
    onSuccess: (result, input) =>
      done(
        result,
        `${title(input.contract)} #${input.contract.id} · ${input.side === "included" ? msg("提供物品") : msg("要求物品")}`,
      ),
  });
  return (
    <>
      <form
        className="market-contract-search"
        onSubmit={(e) => {
          e.preventDefault();
          setFilter(search.trim());
          setSelected(null);
          estimate.reset();
        }}
      >
        <input
          aria-label={msg("搜索合同")}
          placeholder={msg("搜索合同")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          disabled={estimate.isPending}
          maxLength={100}
        />
        <Button
          type="submit"
          variant="outline"
          aria-label={msg("搜索")}
          disabled={estimate.isPending}
        >
          <Search size={18} aria-hidden="true" />
        </Button>
      </form>
      <p className="market-footnote">
        {msg("仅显示未接取且未过期的物品合同；按已同步内容估价。")}
      </p>
      {list.isPending && <p role="status">{msg("正在读取")}</p>}
      {list.isError && (
        <p role="alert">
          {list.error.message}{" "}
          <Button variant="outline" onClick={() => void list.refetch()}>
            {msg("重试")}
          </Button>
        </p>
      )}
      {list.isSuccess && !candidates.length && (
        <p role="status">{msg("当前没有可选合同")}</p>
      )}
      <fieldset className="market-contract-list" disabled={estimate.isPending}>
        <legend className="sr-only">{msg("选择合同")}</legend>
        {candidates.map((c) => (
          <label
            className={
              selected?.id === c.id
                ? "market-contract-row is-selected"
                : "market-contract-row"
            }
            key={c.id}
          >
            <input
              type="radio"
              name="appraisal-contract"
              value={c.id}
              checked={selected?.id === c.id}
              onChange={() => {
                setSelected(c);
                setSide("included");
                estimate.reset();
              }}
            />
            <span>
              <strong>{title(c)}</strong>
              <small>
                #{c.id} · {contractTypes[c.type]} · {msg("到期")}{" "}
                {contractDate(c.date_expired)}
              </small>
            </span>
            <span className="market-contract-price">{amount(c.price)} ISK</span>
          </label>
        ))}
      </fieldset>
      {list.hasNextPage && (
        <Button
          variant="outline"
          disabled={list.isFetchingNextPage || estimate.isPending}
          onClick={() => void list.fetchNextPage()}
        >
          {list.isFetchingNextPage ? msg("正在读取") : msg("加载更多")}
        </Button>
      )}
      <div className="market-contract-actions">
        <Select
          label={msg("估价物品范围")}
          value={side}
          onValueChange={(v) => {
            setSide(v as ContractSide);
            estimate.reset();
          }}
          disabled={estimate.isPending}
          options={[
            { value: "included", label: msg("提供物品") },
            { value: "requested", label: msg("要求物品") },
          ]}
        />
        <Button
          disabled={!selected || estimate.isPending}
          onClick={() =>
            selected && estimate.mutate({ contract: selected, side })
          }
        >
          {estimate.isPending ? msg("正在估价…") : msg("估价")}
        </Button>
      </div>
      {estimate.isError && <p role="alert">{estimate.error.message}</p>}
    </>
  );
}
