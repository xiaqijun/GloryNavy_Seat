import { msg } from "@/lib/i18n";
import { useEffect, useState } from "react";
export function EveSignInButton({ compact = false }: { compact?: boolean }) {
  const [submitting, setSubmitting] = useState(false);
  const [imageFailed, setImageFailed] = useState(false);
  const [imageLoaded, setImageLoaded] = useState(false);
  useEffect(() => {
    const reset = () => setSubmitting(false);
    window.addEventListener("pageshow", reset);
    return () => window.removeEventListener("pageshow", reset);
  }, []);
  return (
    <form
      method="post"
      action="/api/v1/eve/login"
      onSubmit={() => setSubmitting(true)}
    >
      <button
        type="submit"
        className={compact ? "landing-login" : "eve-login-button button-motion"}
        aria-label={msg("使用 EVE Online 登录")}
        disabled={submitting}
        aria-busy={submitting}
      >
        {(compact || !imageLoaded) && (
          <span className={compact ? undefined : "eve-login-label"}>
            {compact ? msg("EVE 登录") : msg("使用 EVE Online 登录")}
          </span>
        )}
        {!compact && !imageFailed && (
          <img
            src="https://web.ccpgamescdn.com/eveonlineassets/developers/eve-sso-login-black-large.png"
            alt=""
            aria-hidden="true"
            data-loaded={imageLoaded}
            onLoad={() => setImageLoaded(true)}
            onError={() => setImageFailed(true)}
          />
        )}
      </button>
      {submitting && (
        <p role="status" className={compact ? "sr-only" : "muted"}>
          {msg("正在前往 EVE Online")}{" "}
        </p>
      )}
    </form>
  );
}
