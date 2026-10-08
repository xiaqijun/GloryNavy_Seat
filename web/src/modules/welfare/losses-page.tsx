import { msg } from "@/lib/i18n";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";
import { Crosshair } from "lucide-react";
import { useSession } from "@/modules/identity";
import { Select } from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { LossBrowser } from "./loss-browser";
import { Apply } from "./page";
import * as api from "./api";
import "./welfare.css";

export default function LossesPage() {
  const s = useSession();
  if (s.isError) return <p role="alert">{s.error.message}</p>;
  if (!s.data) return <p role="status">{msg("正在读取")}</p>;
  if (!s.data.session) return <Navigate to="/" replace />;
  return (
    <Workspace user={s.data.session.user_id} csrf={s.data.session.csrf_token} />
  );
}
function Workspace({ user, csrf }: { user: string; csrf: string }) {
  const [selected, setSelected] = useState("");
  const root = useQuery({
    queryKey: ["welfare", "context", user],
    queryFn: ({ signal }) => api.context("", signal),
  });
  const corp = selected || root.data?.corporations[0]?.id || "";
  return (
    <div className="welfare-page losses-page">
      <header className="welfare-heading">
        <span className="welfare-brand">
          <Crosshair aria-hidden="true" />
        </span>
        <h1>{msg("舰船损失")}</h1>
        {root.data && (
          <Select
            label={msg("军团")}
            value={corp}
            onValueChange={setSelected}
            options={root.data.corporations.map((c) => ({
              value: c.id,
              label: c.name,
            }))}
          />
        )}
      </header>
      {root.isError ? (
        <p role="alert">
          {root.error.message}
          <Button variant="outline" onClick={() => void root.refetch()}>
            {msg("重试")}{" "}
          </Button>
        </p>
      ) : !root.data ? (
        <p role="status">{msg("正在读取")}</p>
      ) : !corp ? (
        <p>{msg("暂无可用军团")}</p>
      ) : (
        <Records
          key={corp}
          corp={corp}
          user={user}
          csrf={csrf}
          characters={root.data.characters}
        />
      )}
    </div>
  );
}
function Records({
  corp,
  user,
  csrf,
  characters,
}: {
  corp: string;
  user: string;
  csrf: string;
  characters: api.Character[];
}) {
  const queryClient = useQueryClient();
  const [claim, setClaim] = useState<{ loss: api.Loss; kind: string } | null>(
    null,
  );
  const [message, setMessage] = useState("");
  const context = useQuery({
    queryKey: ["welfare", "context", user, corp],
    queryFn: ({ signal }) => api.context(corp, signal),
  });
  return (
    <>
      {message && <p role="status">{message}</p>}
      <LossBrowser
        corp={corp}
        csrf={csrf}
        characters={characters}
        select={(loss, kind) => {
          setMessage("");
          setClaim({ loss, kind });
        }}
      />
      {claim &&
        (context.isError ? (
          <p role="alert">
            {context.error.message}
            <Button variant="outline" onClick={() => void context.refetch()}>
              {msg("重试")}{" "}
            </Button>
          </p>
        ) : !context.data ? (
          <p role="status">{msg("正在读取")}</p>
        ) : (
          <Apply
            initialLoss={claim.loss}
            corp={corp}
            csrf={csrf}
            kind={claim.kind}
            context={context.data}
            close={() => setClaim(null)}
            done={() => {
              setClaim(null);
              setMessage(msg("补损申请已提交"));
              void queryClient.invalidateQueries({
                queryKey: ["welfare", "cases"],
              });
              void queryClient.invalidateQueries({
                queryKey: ["welfare", "losses"],
              });
            }}
          />
        ))}
    </>
  );
}
