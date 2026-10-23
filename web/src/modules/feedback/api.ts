import { apiFetch, APIError, getData } from "@/lib/http";
import { msg } from "@/lib/i18n";
export type Kind = "bug" | "feature";
export type Status = "submitted" | "in_progress" | "accepted" | "awaiting_confirmation" | "resolved" | "closed";
export type Screenshot = { id: string; mime: "image/png" | "image/jpeg" };
export type ImageUpload = File;
export type Ticket = {
  accepted_at?: string;
  reward_minor?: number;
  images?: Screenshot[];
  id: string;
  account_id: string;
  author_name: string;
  kind: Kind;
  title: string;
  body?: string;
  status: Status;
  revision: number;
  created_at: string;
  updated_at: string;
};
export type Message = {
  images?: Screenshot[];
  id: string;
  author_name: string;
  staff: boolean;
  body: string;
  status: Status | "";
  created_at: string;
};
export type Detail = {
  ticket: Ticket;
  messages: Message[];
  administrator: boolean;
};
export type Listing = {
  tickets: Ticket[];
  next_before: string;
  administrator: boolean;
};
export type Submission = {
  images?: ImageUpload[];
  request_key: string;
  kind: Kind;
  title: string;
  body: string;
};
export type Reply = {
  images?: ImageUpload[];
  request_key: string;
  revision: number;
  body: string;
  status: Status | "";
};
const object = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object";
const date = (v: unknown) =>
  typeof v === "string" && Number.isFinite(Date.parse(v));
const id = (v: unknown) => typeof v === "string" && /^[1-9]\d*$/.test(v);
const status = (v: unknown) =>
  ["submitted", "in_progress", "accepted", "awaiting_confirmation", "resolved", "closed"].includes(String(v));
const images = (v: unknown) =>
  v === undefined ||
  (Array.isArray(v) &&
    v.length <= 3 &&
    v.every(
      (i) =>
        object(i) &&
        id(i.id) &&
        ["image/png", "image/jpeg"].includes(String(i.mime)),
    ));
export const isTicket = (v: unknown): v is Ticket =>
  object(v) &&
  (v.accepted_at === undefined || date(v.accepted_at)) &&
  (v.reward_minor === undefined || (Number.isSafeInteger(v.reward_minor) && Number(v.reward_minor) >= 0)) &&
  images(v.images) &&
  id(v.id) &&
  typeof v.account_id === "string" &&
  typeof v.author_name === "string" &&
  ["bug", "feature"].includes(String(v.kind)) &&
  typeof v.title === "string" &&
  (v.body === undefined || typeof v.body === "string") &&
  status(v.status) &&
  Number.isSafeInteger(v.revision) &&
  Number(v.revision) > 0 &&
  date(v.created_at) &&
  date(v.updated_at);
const isMessage = (v: unknown): v is Message =>
  object(v) &&
  images(v.images) &&
  id(v.id) &&
  typeof v.author_name === "string" &&
  typeof v.staff === "boolean" &&
  typeof v.body === "string" &&
  (v.status === "" || status(v.status)) &&
  date(v.created_at);
export const list = (
  scope: string,
  kind: string,
  status: string,
  before: string,
  signal?: AbortSignal,
) =>
  getData<Listing>(
    `/api/v1/feedback/tickets?${new URLSearchParams({ scope, kind, status, ...(before ? { before } : {}) })}`,
    (v): v is Listing =>
      object(v) &&
      Array.isArray(v.tickets) &&
      v.tickets.length <= 30 &&
      v.tickets.every(isTicket) &&
      typeof v.administrator === "boolean" &&
      (v.next_before === "" || id(v.next_before)),
    signal,
  );
export const detail = (ticket: string, signal?: AbortSignal) =>
  getData<Detail>(
    `/api/v1/feedback/tickets/${ticket}`,
    (v): v is Detail =>
      object(v) &&
      isTicket(v.ticket) &&
      typeof v.ticket.body === "string" &&
      Array.isArray(v.messages) &&
      v.messages.length <= 100 &&
      v.messages.every(isMessage) &&
      typeof v.administrator === "boolean",
    signal,
  );
async function post(
  path: string,
  csrf: string,
  body: Submission | Reply,
): Promise<Ticket> {
  const { images, ...payload } = body;
  const multipart = new FormData();
  multipart.append("payload", JSON.stringify(payload));
  for (const file of images || []) multipart.append("image", file);
  const response = await apiFetch(`/api/v1/feedback/tickets${path}`, {
    method: "POST",
    credentials: "same-origin",
    headers: images?.length
      ? { "X-CSRF-Token": csrf }
      : { "Content-Type": "application/json", "X-CSRF-Token": csrf },
    body: images?.length ? multipart : JSON.stringify(payload),
  });
  const p = await response.json().catch(() => null);
  if (!response.ok)
    throw new APIError(
      p?.error?.message || msg("请求失败，请重试"),
      response.status,
    );
  if (!isTicket(p?.data)) throw new Error(msg("服务响应格式异常"));
  return p.data;
}
export const create = (csrf: string, body: Submission) => post("", csrf, body);
export const reply = (ticket: string, csrf: string, body: Reply) =>
  post(`/${ticket}/messages`, csrf, body);
