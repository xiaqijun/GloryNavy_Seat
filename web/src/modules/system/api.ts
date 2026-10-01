import { getData } from "@/lib/http";

export interface SystemStatus {
  name: string;
  version: string;
  environment: "tranquility";
  database: "ready";
  schema_version: number;
}

function isSystemStatus(value: unknown): value is SystemStatus {
  if (!value || typeof value !== "object") return false;
  const data = value as Partial<SystemStatus>;
  return (
    data.name === "GloryNavy" &&
    data.environment === "tranquility" &&
    data.database === "ready" &&
    typeof data.schema_version === "number" &&
    typeof data.version === "string"
  );
}

export function getSystemStatus(signal?: AbortSignal): Promise<SystemStatus> {
  return getData("/api/v1/system/status", isSystemStatus, signal);
}
