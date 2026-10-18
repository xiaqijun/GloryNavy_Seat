import { msg } from "@/lib/i18n";
import { useEffect, useRef, useState } from "react";
import { Filter, Save, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { Grant, Permission, Role } from "./manage-api";
import { ScopePicker } from "./scope-picker";

interface DraftGrant extends Grant {
  limited: boolean;
}
export function RoleEditor({
  user,
  role,
  permissions,
  busy,
  error,
  onSave,
  onReload,
}: {
  user: string;
  role: Role;
  permissions: Permission[];
  busy: boolean;
  error: string;
  onSave: (role: Role) => void;
  onReload: () => void;
}) {
  const [name, setName] = useState(role.name);
  const [grants, setGrants] = useState<DraftGrant[]>(() =>
    role.grants
      .filter((g) => permissions.some((p) => p.id === g.permission))
      .map((g) => ({
        ...g,
        limited: g.corporations.length + g.alliances.length > 0,
      })),
  );
  const [errors, setErrors] = useState<Record<string, string>>({});
  const summary = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (error) summary.current?.focus();
  }, [error]);
  const update = (permission: string, patch: Partial<DraftGrant>) =>
    setGrants((gs) =>
      gs.map((g) => (g.permission === permission ? { ...g, ...patch } : g)),
    );
  const groups = [
    { name: msg("平台管理"), test: (p: Permission) => p.scope === "global" },
    {
      name: msg("军团业务"),
      test: (p: Permission) =>
        p.scope === "corporation" && !p.id.endsWith("_division"),
    },
    {
      name: msg("钱包与资产分部"),
      test: (p: Permission) => p.id.endsWith("_division"),
    },
  ];
  return (
    <form
      className="access-editor"
      noValidate
      onSubmit={(e) => {
        e.preventDefault();
        const next: Record<string, string> = {};
        if (!name.trim() || [...name.trim()].length > 80)
          next.name = msg("请输入 1–80 字的角色名称");
        const parsed = grants.map((g) => {
          const corporations = g.limited ? [...new Set(g.corporations)] : [],
            alliances = g.limited ? [...new Set(g.alliances)] : [];
          if (g.limited && corporations.length + alliances.length === 0)
            next[g.permission] = msg(
              "请选择军团或联盟，或选择不限范围",
            );
          if (
            corporations.length + alliances.length > 100 ||
            [...corporations, ...alliances].some(
              (v) => !/^[1-9]\d*$/.test(v) || BigInt(v) > 9223372036854775807n,
            )
          )
            next[g.permission] = msg("最多选择 100 个军团或联盟");
          return { permission: g.permission, corporations, alliances };
        });
        setErrors(next);
        if (Object.keys(next).length) {
          requestAnimationFrame(() => summary.current?.focus());
          return;
        }
        // Editing visible capabilities must not silently remove historical grants.
        const retained = role.grants.filter(
          (g) => !permissions.some((p) => p.id === g.permission),
        );
        onSave({
          ...role,
          name: name.trim(),
          grants: [...retained, ...parsed],
        });
      }}
    >
      <div className="access-section-heading">
        <h2>
          <ShieldCheck size={18} aria-hidden="true" />
          {role.version === "0" ? msg("新建角色") : role.name}
        </h2>
        <span className="muted">
          {grants.length} {msg("项权限")}
        </span>
      </div>
      {(Object.keys(errors).length > 0 || error) && (
        <div
          className="access-form-error"
          role="alert"
          tabIndex={-1}
          ref={summary}
        >
          {Object.entries(errors).map(([key, message]) => (
            <a
              key={key}
              href={`#${key === "name" ? "role-name" : `scope-${key}`}`}
            >
              {message}
            </a>
          ))}
          {error && (
            <>
              <p>{error}</p>
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={onReload}
              >
                {msg("读取最新内容")}{" "}
              </Button>
            </>
          )}
        </div>
      )}
      <div className="access-field">
        <label htmlFor="role-name">{msg("角色名称")}</label>
        <input
          id="role-name"
          value={name}
          maxLength={80}
          disabled={busy}
          aria-invalid={!!errors.name}
          aria-describedby={errors.name ? "role-name-error" : undefined}
          onChange={(e) => setName(e.target.value)}
          autoComplete="off"
        />
        {errors.name && (
          <p id="role-name-error" className="login-error">
            {errors.name}
          </p>
        )}
      </div>
      {groups
        .filter((group) => permissions.some(group.test))
        .map((group, index) => (
          <details
            className="access-permission-group"
            key={group.name}
            open={index < 2}
          >
            <summary>
              {group.name}
              <span className="muted">
                {
                  permissions
                    .filter(group.test)
                    .filter((p) => grants.some((g) => g.permission === p.id))
                    .length
                }
              </span>
            </summary>
            <div className="access-permission-list">
              {permissions.filter(group.test).map((p) => {
                const grant = grants.find((g) => g.permission === p.id);
                return (
                  <div
                    className={`access-permission ${grant ? "is-selected" : ""}`}
                    key={p.id}
                  >
                    <label className="access-check">
                      <input
                        type="checkbox"
                        checked={!!grant}
                        disabled={busy}
                        onChange={(e) =>
                          setGrants((gs) =>
                            e.target.checked
                              ? [
                                  ...gs,
                                  {
                                    permission: p.id,
                                    corporations: [],
                                    alliances: [],
                                    limited: p.scope === "corporation",
                                  },
                                ]
                              : gs.filter((g) => g.permission !== p.id),
                          )
                        }
                      />
                      <span title={p.id}>{p.label}</span>
                    </label>
                    {grant && p.scope === "global" && (
                      <p className="access-scope-note">
                        {msg("可管理全部平台角色和成员授权")}{" "}
                      </p>
                    )}
                    {grant && p.scope === "corporation" && (
                      <div className="access-scope" id={`scope-${p.id}`}>
                        <label className="access-scope-label">
                          <Filter size={14} aria-hidden="true" />
                          <span className="sr-only">{p.label}</span>
                          {msg("授权范围")}{" "}
                          <select
                            aria-label={msg("{0}授权范围", p.label)}
                            value={grant.limited ? "limited" : "all"}
                            disabled={busy}
                            onChange={(e) =>
                              update(p.id, {
                                limited: e.target.value === "limited",
                              })
                            }
                          >
                            <option value="limited">
                              {msg("指定军团 / 联盟")}
                            </option>
                            <option value="all">{msg("不限范围")}</option>
                          </select>
                        </label>
                        {grant.limited && (
                          <ScopePicker user={user} label={p.label} value={grant} disabled={busy}
                            onChange={(scope) => update(p.id, scope)} />
                        )}
                        {errors[p.id] && (
                          <p className="login-error" id={`${p.id}-error`}>
                            {errors[p.id]}
                          </p>
                        )}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          </details>
        ))}
      <div className="access-editor-actions">
        <Button type="submit" disabled={busy} aria-busy={busy}>
          <Save aria-hidden="true" />
          {busy ? msg("正在保存") : msg("保存角色")}
        </Button>
      </div>
    </form>
  );
}
