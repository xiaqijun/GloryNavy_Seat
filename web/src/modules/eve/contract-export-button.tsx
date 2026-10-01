import { msg } from "@/lib/i18n";
import { Download, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { IconAction } from "@/components/ui/icon-action";
import type { Owner } from "./contracts-api";
import {
  collectContracts,
  contractsCSV,
  exportFilename,
} from "./contracts-export";

// The caller keys this component by actor, owner and filters. Unmount aborts an
// in-flight export so a download cannot appear for a previously selected scope.
export function ContractExportButton({
  owner,
  filters,
  disabled,
}: {
  owner: Owner;
  filters: string;
  disabled: boolean;
}) {
  const active = useRef<AbortController | null>(null);
  const [count, setCount] = useState<number | null>(null);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  useEffect(
    () => () => {
      active.current?.abort();
      active.current = null;
    },
    [],
  );

  async function start() {
    if (active.current) {
      active.current.abort();
      active.current = null;
      setCount(null);
      setMessage(msg("导出已取消"));
      return;
    }
    const controller = new AbortController();
    active.current = controller;
    setCount(0);
    setError("");
    setMessage(msg("正在导出合同"));
    try {
      const rows = await collectContracts(
        owner,
        new URLSearchParams(filters),
        controller.signal,
        setCount,
      );
      controller.signal.throwIfAborted();
      if (!rows.length) {
        setError(msg("没有可导出的合同。"));
        setMessage("");
        return;
      }
      const blob = new Blob([contractsCSV(owner, rows)], {
        type: "text/csv;charset=utf-8",
      });
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = exportFilename(owner);
      document.body.append(anchor);
      anchor.click();
      anchor.remove();
      // Keep the object URL alive until the browser has consumed the download.
      window.setTimeout(() => URL.revokeObjectURL(url), 1_000);
      setMessage(msg("已导出 {0} 条合同", rows.length));
    } catch (cause) {
      if (!controller.signal.aborted) {
        setError(
          cause instanceof Error ? cause.message : msg("导出失败，请重试。"),
        );
        setMessage("");
      }
    } finally {
      if (active.current === controller) {
        active.current = null;
        setCount(null);
      }
    }
  }
  return (
    <>
      <IconAction
        className="contract-export"
        label={
          count === null
            ? msg("导出合同（CSV）")
            : msg("取消导出（已读取 {0} 条）", count)
        }
        disabled={disabled && count === null}
        aria-busy={count !== null}
        onClick={() => void start()}
      >
        {count === null ? <Download /> : <X />}
      </IconAction>
      <span className="sr-only" role="status">
        {message}
      </span>
      {error && (
        <p className="contract-export-error" role="alert">
          {error}
        </p>
      )}
    </>
  );
}
