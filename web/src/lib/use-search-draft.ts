import { useState } from "react";

// A URL change replaces the draft; background reads with the same applied query
// leave unfinished input intact. Adjust during render to avoid a stale frame.
export function useSearchDraft(applied: string) {
  const [state, setState] = useState({ applied, draft: applied });
  if (state.applied !== applied) setState({ applied, draft: applied });
  return [
    state.applied === applied ? state.draft : applied,
    (draft: string) => setState({ applied, draft }),
  ] as const;
}

export const normalizeSearch = (value: string) =>
  value.trim().replace(/\s+/gu, " ");
