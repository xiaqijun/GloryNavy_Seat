import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate, useSearchParams } from "react-router-dom";
import { Plus, RefreshCw, Bug, Lightbulb } from "lucide-react";
import { useSession } from "@/modules/identity";
import { msg, getLocale } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { Select } from "@/components/ui/select";
import { Modal } from "@/components/ui/dialog";
import * as api from "./api";
import {
  ScreenshotGallery,
  ScreenshotPicker,
  useScreenshots,
} from "./screenshots";
import "./feedback.css";

const kinds = { bug: msg("BUG 反馈"), feature: msg("功能需求") };
const statuses = {
  submitted: msg("待处理"),
  in_progress: msg("处理中"),
  resolved: msg("已解决"),
  closed: msg("已关闭"),
};
const options = (values: Record<string, string>) =>
  Object.entries(values).map(([value, label]) => ({ value, label }));
const time = (value: string) =>
  new Date(value).toLocaleString(getLocale(), {
    dateStyle: "medium",
    timeStyle: "short",
  });
export default function FeedbackPage() {
  const s = useSession();
  if (s.isError) return <p role="alert">{s.error.message}</p>;
  if (!s.data) return <p role="status">{msg("正在读取")}</p>;
  if (!s.data.session) return <Navigate to="/" replace />;
  return (
    <Workspace
      key={s.data.session.user_id}
      user={s.data.session.user_id}
      csrf={s.data.session.csrf_token}
    />
  );
}
function Workspace({ user, csrf }: { user: string; csrf: string }) {
  const [scope, setScope] = useState("mine"),
    [kind, setKind] = useState(""),
    [status, setStatus] = useState(""),
    [pages, setPages] = useState<string[]>([""]),
    [compose, setCompose] = useState(false);
  const [params, setParams] = useSearchParams();
  const selected = params.get("ticket");
  const qc = useQueryClient();
  const before = pages.at(-1) || "";
  const q = useQuery({
    queryKey: ["feedback", "list", user, scope, kind, status, before],
    queryFn: ({ signal }) => api.list(scope, kind, status, before, signal),
  });
  const filter = (set: (v: string) => void, v: string) => {
    set(v);
    setPages([""]);
  };
  const open = (id: string) => setParams({ ticket: id });
  const refresh = () => qc.invalidateQueries({ queryKey: ["feedback"] });
  return (
    <section className="feedback-page">
      <header className="feedback-heading">
        <h1>{msg("意见反馈")}</h1>
        <Button onClick={() => setCompose(true)}>
          <Plus />
          {msg("提交反馈")}
        </Button>
      </header>
      <div className="feedback-filters">
        {q.data?.administrator && (
          <Select
            label={msg("反馈范围")}
            value={scope}
            onValueChange={(v) => filter(setScope, v)}
            options={[
              { value: "mine", label: msg("我的反馈") },
              { value: "all", label: msg("全部反馈") },
            ]}
          />
        )}
        <Select
          label={msg("反馈类型")}
          value={kind}
          onValueChange={(v) => filter(setKind, v)}
          options={[{ value: "", label: msg("全部类型") }, ...options(kinds)]}
        />
        <Select
          label={msg("处理状态")}
          value={status}
          onValueChange={(v) => filter(setStatus, v)}
          options={[
            { value: "", label: msg("全部状态") },
            ...options(statuses),
          ]}
        />
        <IconAction
          label={msg("刷新")}
          disabled={q.isFetching}
          onClick={() => void refresh()}
        >
          <RefreshCw size={18} />
        </IconAction>
      </div>
      {q.isError ? (
        <div className="feedback-error" role="alert">
          {q.error.message}
          <Button variant="outline" onClick={() => void q.refetch()}>
            {msg("重试")}
          </Button>
        </div>
      ) : !q.data ? (
        <p role="status">{msg("正在读取")}</p>
      ) : (
        <>
          {q.data.tickets.length === 0 ? (
            <div className="feedback-empty">{msg("暂无反馈")}</div>
          ) : (
            <div className="feedback-list">
              {q.data.tickets.map((t) => (
                <button
                  className="feedback-row"
                  key={t.id}
                  onClick={() => open(t.id)}
                >
                  <span className="feedback-kind">
                    {t.kind === "bug" ? (
                      <Bug size={18} />
                    ) : (
                      <Lightbulb size={18} />
                    )}
                    <span>{kinds[t.kind]}</span>
                  </span>
                  <span className="feedback-subject">
                    <strong>{t.title}</strong>
                    <span>
                      #{t.id} · {t.author_name} · {time(t.created_at)}
                    </span>
                  </span>
                  <span className="feedback-status" data-status={t.status}>
                    {statuses[t.status]}
                  </span>
                </button>
              ))}
            </div>
          )}
          {(pages.length > 1 || q.data.next_before) && (
            <nav className="feedback-pagination" aria-label={msg("分页")}>
              <Button
                variant="outline"
                disabled={pages.length === 1}
                onClick={() => setPages((p) => p.slice(0, -1))}
              >
                {msg("上一页")}
              </Button>
              <Button
                variant="outline"
                disabled={!q.data.next_before}
                onClick={() => setPages((p) => [...p, q.data.next_before])}
              >
                {msg("下一页")}
              </Button>
            </nav>
          )}
        </>
      )}
      {compose && (
        <Compose
          csrf={csrf}
          close={() => setCompose(false)}
          done={(t) => {
            setCompose(false);
            void refresh();
            open(t.id);
          }}
        />
      )}
      {selected && /^[1-9]\d*$/.test(selected) && (
        <Details
          key={selected}
          id={selected}
          user={user}
          csrf={csrf}
          close={() => setParams({})}
          updated={refresh}
        />
      )}
    </section>
  );
}
function Compose({
  csrf,
  close,
  done,
}: {
  csrf: string;
  close: () => void;
  done: (t: api.Ticket) => void;
}) {
  const [kind, setKind] = useState<api.Kind>("bug"),
    [title, setTitle] = useState(""),
    [body, setBody] = useState("");
  const [key, setKey] = useState(() => crypto.randomUUID());
  const mutation = useMutation({
    mutationFn: () =>
      api.create(csrf, {
        request_key: key,
        kind,
        title,
        body,
        images: screenshots.images,
      }),
    onSuccess: done,
  });
  const edit = () => {
    setKey(crypto.randomUUID());
    mutation.reset();
  };
  const screenshots = useScreenshots(edit, mutation.isPending);
  return (
    <Modal
      title={msg("提交反馈")}
      close={close}
      busy={mutation.isPending || screenshots.busy}
      className="feedback-modal"
      footer={
        <Button
          form="feedback-create"
          type="submit"
          disabled={
            mutation.isPending ||
            screenshots.busy ||
            !title.trim() ||
            !body.trim()
          }
        >
          {mutation.isPending ? msg("正在提交") : msg("提交")}
        </Button>
      }
    >
      <form
        id="feedback-create"
        className="feedback-form"
        onPaste={screenshots.paste}
        onSubmit={(e) => {
          e.preventDefault();
          if (!mutation.isPending && !screenshots.busy) mutation.mutate();
        }}
      >
        <label>
          {msg("反馈类型")}
          <Select
            label={msg("反馈类型")}
            value={kind}
            onValueChange={(v) => {
              setKind(v as api.Kind);
              edit();
            }}
            disabled={mutation.isPending}
            options={options(kinds)}
          />
        </label>
        <label>
          {msg("标题")}
          <input
            autoFocus
            required
            maxLength={120}
            value={title}
            onChange={(e) => {
              setTitle(e.target.value);
              edit();
            }}
            disabled={mutation.isPending}
          />
        </label>
        <label>
          {msg("详细描述")}
          <textarea
            required
            rows={8}
            maxLength={6000}
            value={body}
            onChange={(e) => {
              setBody(e.target.value);
              edit();
            }}
            disabled={mutation.isPending}
            placeholder={
              kind === "bug"
                ? msg("在哪个页面、做了什么操作、出现什么问题？")
                : msg("希望增加什么功能？")
            }
          />
        </label>
        <ScreenshotPicker value={screenshots} disabled={mutation.isPending} />
        {mutation.isError && <p role="alert">{mutation.error.message}</p>}
      </form>
    </Modal>
  );
}
function Details({
  id,
  user,
  csrf,
  close,
  updated,
}: {
  id: string;
  user: string;
  csrf: string;
  close: () => void;
  updated: () => Promise<unknown>;
}) {
  const q = useQuery({
    queryKey: ["feedback", "detail", user, id],
    queryFn: ({ signal }) => api.detail(id, signal),
  });
  return (
    <Modal
      title={`${msg("反馈详情")} #${id}`}
      close={close}
      className="feedback-modal"
    >
      {q.isError ? (
        <div className="feedback-error" role="alert">
          {q.error.message}
          <Button variant="outline" onClick={() => void q.refetch()}>
            {msg("重试")}
          </Button>
        </div>
      ) : !q.data ? (
        <p role="status">{msg("正在读取")}</p>
      ) : (
        <Thread data={q.data} csrf={csrf} updated={updated} />
      )}
    </Modal>
  );
}
function Thread({
  data,
  csrf,
  updated,
}: {
  data: api.Detail;
  csrf: string;
  updated: () => Promise<unknown>;
}) {
  const [body, setBody] = useState(""),
    [status, setStatus] = useState<api.Status | "">(""),
    [key, setKey] = useState(() => crypto.randomUUID());
  const t = data.ticket;
  const mutation = useMutation({
    mutationFn: () =>
      api.reply(t.id, csrf, {
        request_key: key,
        revision: t.revision,
        body,
        status,
        images: screenshots.images,
      }),
    onSuccess: async () => {
      setBody("");
      setStatus("");
      screenshots.clear();
      setKey(crypto.randomUUID());
      await updated();
    },
  });
  const edit = () => {
    setKey(crypto.randomUUID());
    mutation.reset();
  };
  const screenshots = useScreenshots(edit, mutation.isPending);
  return (
    <div className="feedback-thread">
      <div className="feedback-detail-heading">
        <span>{kinds[t.kind]}</span>
        <span className="feedback-status" data-status={t.status}>
          {statuses[t.status]}
        </span>
      </div>
      <h2>{t.title}</h2>
      <p className="feedback-meta">
        {t.author_name} · {time(t.created_at)}
      </p>
      <p className="feedback-content">{t.body}</p>
      <ScreenshotGallery ticket={t.id} images={t.images} />
      {data.messages.length > 0 && (
        <ol className="feedback-messages">
          {data.messages.map((m) => (
            <li key={m.id}>
              <div className="feedback-message-meta">
                <strong>{m.author_name}</strong>
                {m.staff && <span>{msg("管理员")}</span>}
                <time>{time(m.created_at)}</time>
              </div>
              {m.status && <p>{msg("状态变更为 {0}", statuses[m.status])}</p>}
              {m.body && <p className="feedback-content">{m.body}</p>}
              <ScreenshotGallery ticket={t.id} images={m.images} />
            </li>
          ))}
        </ol>
      )}
      {data.messages.length >= 100 ? (
        <p>{msg("此反馈的回复已达上限，请提交新的反馈")}</p>
      ) : (
        <form
          className="feedback-form feedback-reply"
          onPaste={screenshots.paste}
          onSubmit={(e) => {
            e.preventDefault();
            if (!mutation.isPending && !screenshots.busy) mutation.mutate();
          }}
        >
          {data.administrator && (
            <label>
              {msg("处理状态")}
              <Select
                label={msg("处理状态")}
                value={status}
                onValueChange={(v) => {
                  setStatus(v as api.Status | "");
                  edit();
                }}
                disabled={mutation.isPending}
                options={[
                  { value: "", label: msg("保持当前状态") },
                  ...options(statuses),
                ]}
              />
            </label>
          )}
          <label>
            {data.administrator ? msg("处理回复") : msg("补充说明")}
            <textarea
              maxLength={2000}
              rows={3}
              value={body}
              onChange={(e) => {
                setBody(e.target.value);
                edit();
              }}
              disabled={mutation.isPending}
            />
          </label>
          <ScreenshotPicker value={screenshots} disabled={mutation.isPending} />
          {mutation.isError && (
            <div className="feedback-error" role="alert">
              {mutation.error.message}
              <Button
                type="button"
                variant="outline"
                onClick={async () => {
                  await updated();
                  setKey(crypto.randomUUID());
                  mutation.reset();
                }}
              >
                {msg("刷新")}
              </Button>
            </div>
          )}
          <Button
            type="submit"
            disabled={
              mutation.isPending ||
              screenshots.busy ||
              (!body.trim() && !status)
            }
          >
            {mutation.isPending
              ? msg("正在提交")
              : data.administrator
                ? msg("保存处理")
                : msg("发送回复")}
          </Button>
        </form>
      )}
    </div>
  );
}
