declare module "@eveshipfit/dogma-engine/esf_dogma_engine_bg.js" {
  export function __wbg_set_wasm(exports: WebAssembly.Exports): void;
  export function load_sde(bytes: Uint8Array): number;
  export function calculate(fit: unknown, options: unknown): unknown;
}
