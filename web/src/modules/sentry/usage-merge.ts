import * as api from "./api";

const maxUsageDisplayGapMs = 5 * 60 * 1000;

export type UsageRecord = {
  system_key: string;
  system_name: string;
  started_at: string;
  ended_at: string;
  duration_seconds: number;
  charge_minor: number;
  reward_minor: number;
  charge_state?: api.AlertConsumption["state"];
  charge_returned?: boolean;
};

type UsageSource = {
  kind: "charge" | "reward";
  system_id?: string;
  system_name?: string;
  started_at: string;
  ended_at: string;
  duration_seconds: number;
  coins_minor: number;
  state?: api.AlertConsumption["state"];
};

const usageSystemKey = (systemID?: string, systemName?: string) => {
  const id = (systemID || "").trim().toLowerCase().replace(/^legacy:/, "");
  if (id) return id;
  const name = (systemName || "").trim().toLowerCase();
  return name ? `name:${name}` : "unknown";
};

export function mergeUsageRecords(consumptions: api.AlertConsumption[], rewards: api.MonitorRewardRecord[]): UsageRecord[] {
  const sources: UsageSource[] = [
    ...consumptions.map((item) => ({
      kind: "charge" as const,
      system_id: item.system_id,
      system_name: item.system_id,
      started_at: item.started_at,
      ended_at: item.ended_at,
      duration_seconds: item.duration_seconds,
      coins_minor: item.coins_minor,
      state: item.state,
    })),
    ...rewards.map((item) => ({
      kind: "reward" as const,
      system_id: item.system_id,
      system_name: item.system_name,
      started_at: item.started_at,
      ended_at: item.ended_at,
      duration_seconds: item.duration_seconds,
      coins_minor: item.coins_minor,
    })),
  ].sort((a, b) => Date.parse(a.started_at) - Date.parse(b.started_at) || Date.parse(a.ended_at) - Date.parse(b.ended_at));

  const groups = new Map<string, UsageRecord[]>();
  for (const source of sources) {
    const key = usageSystemKey(source.system_id, source.system_name);
    const group = groups.get(key) || [];
    const previous = group[group.length - 1];
    const sourceStart = Date.parse(source.started_at);
    const sourceEnd = Date.parse(source.ended_at);
    const previousEnd = previous ? Date.parse(previous.ended_at) : 0;
    if (previous && sourceStart <= previousEnd + maxUsageDisplayGapMs) {
      if (sourceEnd > previousEnd) previous.ended_at = source.ended_at;
      previous.duration_seconds = Math.max(1, Math.round((Date.parse(previous.ended_at) - Date.parse(previous.started_at)) / 1000));
      if (source.kind === "reward" && source.system_name) previous.system_name = source.system_name;
      if (source.kind === "charge") {
        previous.charge_minor += source.coins_minor;
        previous.charge_state = previous.charge_state === undefined || previous.charge_state === source.state ? source.state : undefined;
        const returned = source.state === "released" || source.state === "refunded";
        previous.charge_returned = previous.charge_returned === undefined || previous.charge_returned === returned ? returned : undefined;
      } else {
        previous.reward_minor += source.coins_minor;
      }
      groups.set(key, group);
      continue;
    }
    group.push({
      system_key: key,
      system_name: source.system_name || source.system_id || "—",
      started_at: source.started_at,
      ended_at: source.ended_at,
      duration_seconds: source.duration_seconds,
      charge_minor: source.kind === "charge" ? source.coins_minor : 0,
      reward_minor: source.kind === "reward" ? source.coins_minor : 0,
      charge_state: source.kind === "charge" ? source.state : undefined,
      charge_returned: source.kind === "charge" ? source.state === "released" || source.state === "refunded" : undefined,
    });
    groups.set(key, group);
  }
  return [...groups.values()].flat().sort((a, b) => Date.parse(b.started_at) - Date.parse(a.started_at));
}
