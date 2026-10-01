import { gameLabels, gameTerm } from "@/lib/eve-terminology";

// Protocol keys stay unchanged; names come from the pinned CCP source catalog.
export const journalTypes: Record<string, string> = gameLabels("wallet");
export const contextTypes: Record<string, string> = gameLabels("contexts");
export const contextLabel = (code?: string) =>
  code ? gameTerm("contexts", code) : "";
