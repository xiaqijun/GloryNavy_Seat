import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { apiFetch, APIError, getData } from "@/lib/http";
import { msg } from "@/lib/i18n";
import { minor } from "@/modules/welfare/api";
import { Button } from "@/components/ui/button";
import { Modal } from "@/components/ui/dialog";

type Config = { amount_minor: number; version: number };
const isConfig = (v: unknown): v is Config =>
  !!v &&
  typeof v === "object" &&
  "amount_minor" in v &&
  Number.isSafeInteger(v.amount_minor) &&
  Number(v.amount_minor) >= 0 &&
  "version" in v &&
  Number.isSafeInteger(v.version);
export function RewardSettings({
  csrf,
  close,
}: {
  csrf: string;
  close: () => void;
}) {
  const q = useQuery({
    queryKey: ["feedback", "reward-settings"],
    queryFn: ({ signal }) =>
      getData("/api/v1/feedback/reward-settings", isConfig, signal),
  });
  return (
    <Modal title={msg("反馈奖励设置")} close={close}>
      {q.isError ? (
        <p role="alert">{q.error.message}</p>
      ) : !q.data ? (
        <p role="status">{msg("正在读取")}</p>
      ) : (
        <ConfigForm
          key={q.data.version}
          config={q.data}
          csrf={csrf}
          saved={close}
        />
      )}
    </Modal>
  );
}
function ConfigForm({
  config,
  csrf,
  saved,
}: {
  config: Config;
  csrf: string;
  saved: () => void;
}) {
  const [amount, setAmount] = useState(String(config.amount_minor / 100));
  const [key, setKey] = useState(() => crypto.randomUUID());
  const value = (() => {
    try {
      return minor(amount);
    } catch {
      return NaN;
    }
  })();
  const mutation = useMutation({
    mutationFn: async () => {
      const r = await apiFetch("/api/v1/feedback/reward-settings", {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
        body: JSON.stringify({
          amount_minor: value,
          version: config.version,
          request_key: key,
        }),
      });
      const result = await r.json();
      if (!r.ok)
        throw new APIError(
          result?.error?.message || msg("请求失败，请重试"),
          r.status,
        );
    },
    onSuccess: saved,
  });
  return (
    <form
      className="feedback-form"
      onSubmit={(e) => {
        e.preventDefault();
        if (
          !mutation.isPending &&
          Number.isSafeInteger(value) &&
          value >= 0 &&
          value <= 1e12
        )
          mutation.mutate();
      }}
    >
      <label>
        {msg("采纳奖励 / 果壳币")}
        <input
          inputMode="decimal"
          value={amount}
          disabled={mutation.isPending}
          onChange={(e) => {
            setAmount(e.target.value);
            setKey(crypto.randomUUID());
            mutation.reset();
          }}
        />
      </label>
      <p>{msg("设为 0 不发放奖励")}</p>
      {amount !== "" && !Number.isSafeInteger(value) && <p role="alert">{msg("金额最多保留两位小数")}</p>}
      {Number.isSafeInteger(value) && value > 1e12 && <p role="alert">{msg("金额超出范围")}</p>}
      {mutation.isError && <p role="alert">{mutation.error.message}</p>}
      <Button
        type="submit"
        disabled={
          mutation.isPending ||
          !Number.isSafeInteger(value) ||
          value < 0 ||
          value > 1e12
        }
      >
        {msg("保存")}
      </Button>
    </form>
  );
}
