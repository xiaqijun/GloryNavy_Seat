import { getData } from "@/lib/http";

export type Requirement = {
  source: "alliance";
  monthly_points: number;
  version: string;
  can_manage: boolean;
};

export const getPAPRequirement = (signal?: AbortSignal) =>
  getData(
    "/api/v1/attendance/pap-requirement",
    (v: unknown): v is Requirement => {
      if (!v || typeof v !== "object") return false;
      const r = v as Requirement;
      return (
        r.source === "alliance" &&
        Number.isInteger(r.monthly_points) &&
        r.monthly_points >= 1 &&
        r.monthly_points <= 100000 &&
        typeof r.version === "string" &&
        /^[1-9]\d*$/.test(r.version) &&
        typeof r.can_manage === "boolean"
      );
    },
    signal,
  );

export type AllianceReport = {
  source: "alliance";
  month: string;
  points: number;
  available: boolean;
  complete: boolean;
  state: "idle" | "syncing" | "ready" | "error";
  records_total: number;
  last_synced_at: string | null;
  characters: Array<{
    character_id: number;
    character_name: string;
    pap: number;
  }>;
  version: string;
  can_manage: boolean;
};

const object = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";

export const isAllianceReport = (v: unknown): v is AllianceReport => {
  if (!object(v)) return false;
  const r = v as AllianceReport;
  return (
    r.source === "alliance" &&
    typeof r.month === "string" &&
    Number.isFinite(r.points) &&
    typeof r.available === "boolean" &&
    typeof r.complete === "boolean" &&
    ["idle", "syncing", "ready", "error"].includes(r.state) &&
    Number.isInteger(r.records_total) &&
    (r.last_synced_at === null || typeof r.last_synced_at === "string") &&
    typeof r.version === "string" &&
    /^[1-9]\d*$/.test(r.version) &&
    typeof r.can_manage === "boolean" &&
    Array.isArray(r.characters) &&
    r.characters.every(
      (c) =>
        !!c &&
        Number.isSafeInteger(c.character_id) &&
        c.character_id > 0 &&
        typeof c.character_name === "string" &&
        c.character_name.length > 0 &&
        Number.isFinite(c.pap) &&
        c.pap >= 0,
    )
  );
};

export const getAlliancePAP = (signal?: AbortSignal) =>
  getData("/api/v1/attendance/alliance-pap", isAllianceReport, signal);

export type AlliancePAPMember = {
  user_id: string;
  name: string;
  points: number;
  achieved: boolean;
  characters: Array<{
    character_id: number;
    character_name: string;
    pap: number;
  }>;
};

export type AlliancePAPMembersReport = {
  source: "alliance";
  month: string;
  target: number;
  available: boolean;
  complete: boolean;
  state: "idle" | "syncing" | "ready" | "error";
  records_total: number;
  last_synced_at: string | null;
  version: string;
  members: AlliancePAPMember[];
};

export const isAlliancePAPMembersReport = (v: unknown): v is AlliancePAPMembersReport => {
  if (!object(v)) return false;
  const r = v as AlliancePAPMembersReport;
  return (
    r.source === "alliance" &&
    typeof r.month === "string" &&
    /^\d{4}-\d{2}$/.test(r.month) &&
    Number.isInteger(r.target) && r.target >= 1 &&
    typeof r.available === "boolean" &&
    typeof r.complete === "boolean" &&
    ["idle", "syncing", "ready", "error"].includes(r.state) &&
    Number.isInteger(r.records_total) && r.records_total >= 0 &&
    (r.last_synced_at === null || typeof r.last_synced_at === "string") &&
    typeof r.version === "string" && /^[1-9]\d*$/.test(r.version) &&
    Array.isArray(r.members) && r.members.every((member) =>
      !!member &&
      typeof member.user_id === "string" &&
      typeof member.name === "string" &&
      Number.isFinite(member.points) && member.points >= 0 &&
      typeof member.achieved === "boolean" &&
      Array.isArray(member.characters) && member.characters.every((character) =>
        !!character && Number.isSafeInteger(character.character_id) && character.character_id > 0 &&
        typeof character.character_name === "string" && character.character_name.length > 0 &&
        Number.isFinite(character.pap) && character.pap >= 0,
      ),
    )
  );
};

export const getAlliancePAPMembers = (month: string, signal?: AbortSignal) =>
  getData(`/api/v1/attendance/alliance-pap/members?month=${encodeURIComponent(month)}`, isAlliancePAPMembersReport, signal);

export const getAlliancePAPMemberMonths = (signal?: AbortSignal) =>
  getData(
    "/api/v1/attendance/alliance-pap/member-months",
    (v: unknown): v is { months: string[] } =>
      object(v) && Array.isArray(v.months) && v.months.every((month) => typeof month === "string" && /^\d{4}-\d{2}$/.test(month)),
    signal,
  );

export type AlliancePAPConversionMonth = {
  month: string;
  version: string;
  token: string;
  mode: "manual" | "automatic";
  points: number;
  converted: number;
  pending: number;
  coins_minor: number;
  characters: number;
  unit_scale: number;
};

const isConversionMonth = (v: unknown): v is AlliancePAPConversionMonth => {
  if (!object(v)) return false;
  const r = v as Record<string, unknown>;
  return (
    typeof r.month === "string" && /^\d{4}-\d{2}$/.test(r.month) &&
    typeof r.version === "string" && /^[1-9]\d*$/.test(r.version) &&
    typeof r.token === "string" && /^[a-f0-9]{64}$/.test(r.token) &&
    ["manual", "automatic"].includes(String(r.mode)) &&
    ["points", "converted", "pending", "coins_minor", "characters", "unit_scale"].every(
      (k) => Number.isSafeInteger(r[k]) && Number(r[k]) >= 0,
    ) && Number(r.unit_scale) > 0
  );
};

export const isAlliancePAPConversions = (v: unknown): v is { months: AlliancePAPConversionMonth[] } =>
  object(v) && Array.isArray(v.months) && v.months.every(isConversionMonth);

export const getAlliancePAPConversions = (signal?: AbortSignal) =>
  getData("/api/v1/attendance/alliance-pap/conversions", isAlliancePAPConversions, signal);

export type AlliancePAPFulfillment = {
  source: "alliance";
  month: string;
  target: number;
  eligible_accounts: number;
  achieved_accounts: number;
  rate_bps: number;
  available: boolean;
  state: "idle" | "syncing" | "ready" | "error";
  last_synced_at: string | null;
  version: string;
};

export const isAlliancePAPFulfillment = (v: unknown): v is AlliancePAPFulfillment => {
  if (!object(v)) return false;
  const r = v as AlliancePAPFulfillment;
  return (
    r.source === "alliance" &&
    typeof r.month === "string" &&
    Number.isInteger(r.target) && r.target >= 1 &&
    Number.isInteger(r.eligible_accounts) && r.eligible_accounts >= 0 &&
    Number.isInteger(r.achieved_accounts) && r.achieved_accounts >= 0 &&
    Number.isInteger(r.rate_bps) && r.rate_bps >= 0 && r.rate_bps <= 10000 &&
    typeof r.available === "boolean" &&
    ["idle", "syncing", "ready", "error"].includes(r.state) &&
    (r.last_synced_at === null || typeof r.last_synced_at === "string") &&
    typeof r.version === "string" && /^[1-9]\d*$/.test(r.version)
  );
};

export const getAlliancePAPFulfillment = (signal?: AbortSignal) =>
  getData("/api/v1/attendance/alliance-pap/summary", isAlliancePAPFulfillment, signal);
