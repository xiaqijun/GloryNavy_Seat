import { msg } from "@/lib/i18n";
import * as glue from "@eveshipfit/dogma-engine/esf_dogma_engine_bg.js";
import wasmURL from "@eveshipfit/dogma-engine/esf_dogma_engine_bg.wasm?url";
import sdeURL from "@eveshipfit/sde/dist/sde.dat?url";
import namesURL from "@eveshipfit/sde/dist/names.dat?url";
import { readCatalog, readNames, type Catalog } from "./sde";
import { importEFT, exportEFT, engineInput, type Fit } from "./model";
import { summarize } from "./statistics";
let catalog: Catalog;
let names: Map<string, number> | undefined;
async function bytes(url: string) {
  const r = await fetch(url);
  if (!r.ok) throw Error(msg("模拟数据加载失败"));
  return new Uint8Array(await r.arrayBuffer());
}
const ready = (async () => {
  const [wasm, sde] = await Promise.all([bytes(wasmURL), bytes(sdeURL)]);
  const { instance } = await WebAssembly.instantiate(wasm, {
    "./esf_dogma_engine_bg.js": glue,
  });
  glue.__wbg_set_wasm(instance.exports);
  (instance.exports.__wbindgen_start as () => void)();
  const build = glue.load_sde(sde);
  catalog = readCatalog(sde);
  if (build !== catalog.build) throw Error(msg("模拟数据版本不一致"));
  return catalog;
})();
void ready.catch(() => {});
self.onmessage = async (
  e: MessageEvent<{
    id: number;
    action: string;
    fit: Fit;
    text?: string;
    skills?: Record<string, number>;
  }>,
) => {
  const m = e.data;
  try {
    await ready;
    let data: unknown;
    if (m.action === "init") data = catalog;
    else if (m.action === "import") {
      if (!names) {
        const n = readNames(await bytes(namesURL));
        if (n.build !== catalog.build) throw Error(msg("名称数据版本不一致"));
        names = n.names;
      }
      data = importEFT(m.text ?? "", catalog, names);
    } else if (m.action === "export") data = exportEFT(m.fit, catalog);
    else {
      data = summarize(
        glue.calculate(engineInput(m.fit, catalog, m.skills), null),
        m.fit,
        catalog,
      );
    }
    self.postMessage({ id: m.id, data });
  } catch (err) {
    self.postMessage({
      id: m.id,
      error: err instanceof Error ? err.message : msg("配装计算失败"),
    });
  }
};
