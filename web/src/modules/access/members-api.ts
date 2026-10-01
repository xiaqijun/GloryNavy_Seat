import { getData } from "@/lib/http";
import { isAccountAccess, type AccountAccess } from "./api";
import type { BoundCharacter } from "@/modules/identity/api";

type Binding = {
  value: string;
  confirmation: "unfilled" | "pending" | "confirmed";
};
export type MemberData = {
  user_id: string;
  characters: BoundCharacter[];
  access: AccountAccess;
  community: { qq: Binding; kook: Binding } | null;
};
const record = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const binding = (v: unknown): v is Binding =>
  record(v) &&
  typeof v.value === "string" &&
  ["unfilled", "pending", "confirmed"].includes(String(v.confirmation));
export function isMemberData(v: unknown): v is MemberData {
  return (
    record(v) &&
    typeof v.user_id === "string" &&
    isAccountAccess(v.access) &&
    Array.isArray(v.characters) &&
    v.characters.every(
      (c) =>
        record(c) &&
        typeof c.id === "string" &&
        /^[1-9]\d*$/.test(c.id) &&
        typeof c.name === "string" &&
        ["active", "blocked"].includes(String(c.status)) &&
        typeof c.is_main === "boolean",
    ) &&
    (v.community === null ||
      (record(v.community) &&
        binding(v.community.qq) &&
        binding(v.community.kook)))
  );
}
export const getMemberData = (user: string, signal?: AbortSignal) =>
  getData(
    `/api/v1/access/members/${encodeURIComponent(user)}/data`,
    isMemberData,
    signal,
  );
