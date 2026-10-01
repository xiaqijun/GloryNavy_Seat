import { useState } from "react";
import { Building2, UserRound, Package } from "lucide-react";
import { cn } from "@/lib/utils";

// Decorative next to a visible name; failed CDN requests retain the same space.
export function EveImage({
  id,
  kind,
  variation = "icon",
  className,
}: {
  id: string;
  kind: "character" | "corporation" | "type";
  variation?: "icon" | "render";
  className?: string;
}) {
  const valid = /^[1-9][0-9]*$/.test(id);
  const category = kind === "character" ? "characters" : kind === "type" ? "types" : "corporations";
  const typeIcon = valid && kind === "type" ? `https://images.evetech.net/types/${id}/icon?size=64&tenant=tranquility` : "";
  const render = valid && kind === "type" && variation === "render" ? `https://images.evetech.net/types/${id}/render?size=128&tenant=tranquility` : "";
  const [failed, setFailed] = useState("");
  const [loaded, setLoaded] = useState("");
  const src = render && failed !== render
    ? render
    : kind === "type"
      ? typeIcon
      : valid
        ? `https://images.evetech.net/${category}/${id}/${kind === "character" ? "portrait" : "logo"}?size=256&tenant=tranquility`
        : "";
  const Fallback = kind === "character" ? UserRound : kind === "type" ? Package : Building2;
  return (
    <span className={cn("eve-image", className)} aria-hidden="true">
      {(!src || failed === src || loaded !== src) && <Fallback />}
      {src && failed !== src && (
        <img
          src={src}
          alt=""
          width={256}
          height={256}
          decoding="async"
          loading="lazy"
          style={{ opacity: loaded === src ? 1 : 0 }}
          onLoad={() => setLoaded(src)}
          onError={() => setFailed(src)}
        />
      )}
    </span>
  );
}
