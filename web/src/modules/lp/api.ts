import { getData } from "@/lib/http";
export type Name = { zh: string; en: string };
export type Catalog = { build: number; corporations: (Name & { id: number; faction_id: number })[]; factions: (Name & { id: number })[] };
export type Material = { type_id: number; quantity: number };
export type Offer = { offer_id: number; type_id: number; quantity: number; lp_cost: number; ak_cost: number; isk_cost: number; required_items: Material[]; blueprint?: { product: Material; materials: Material[] } };
export type Offers = { corporation_id: number; sde_build: number; offers: Offer[]; names: Record<string, Name>; observed_at: string };
export type Price = { buy: number | null; sell: number | null };
export type Prices = { items: Record<string, Price>; observed_at: string; source?: string };
const object = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const number = (v: unknown): v is number => typeof v === "number" && Number.isFinite(v) && v >= 0 && v <= 9e15;
const integer = (v: unknown): v is number => number(v) && Number.isSafeInteger(v);
const name = (v: unknown): v is Name => object(v) && typeof v.zh === "string" && typeof v.en === "string";
const material = (v: unknown): v is Material => object(v) && integer(v.type_id) && integer(v.quantity);
export function validCatalog(v: unknown): v is Catalog {
  return object(v) && integer(v.build) && Array.isArray(v.corporations) && v.corporations.every(c => object(c) && integer(c.id) && integer(c.faction_id) && name(c)) && Array.isArray(v.factions) && v.factions.every(f => object(f) && integer(f.id) && name(f));
}
export function validOffers(v: unknown): v is Offers {
  if (!object(v) || !integer(v.corporation_id) || !integer(v.sde_build) || typeof v.observed_at !== "string" || !object(v.names) || !Object.values(v.names).every(name) || !Array.isArray(v.offers)) return false;
  return v.offers.every(o => object(o) && [o.offer_id,o.type_id,o.quantity,o.lp_cost,o.ak_cost].every(integer) && number(o.isk_cost) && Array.isArray(o.required_items) && o.required_items.every(material) && (o.blueprint === undefined || (object(o.blueprint) && material(o.blueprint.product) && Array.isArray(o.blueprint.materials) && o.blueprint.materials.every(material))));
}
export function validPrices(v: unknown): v is Prices {
  return object(v) && typeof v.observed_at === "string" && Number.isFinite(Date.parse(v.observed_at)) && object(v.items) && Object.values(v.items).every(p => object(p) && [p.buy,p.sell].every(x => x === null || (number(x) && x > 0)));
}
export const catalog = (signal?: AbortSignal) => getData("/api/v1/lp/catalog",validCatalog,signal);
export const offers = (corporation: string, signal?: AbortSignal) => getData(`/api/v1/lp/offers?corporation=${encodeURIComponent(corporation)}`,validOffers,signal);
export async function prices(region: string, offers: Offer[], signal?: AbortSignal): Promise<Prices> {
  const ids = [...new Set(offers.flatMap(o => [...(o.blueprint ? [o.blueprint.product.type_id,...o.blueprint.materials.map(i=>i.type_id)] : [o.type_id]),...o.required_items.map(i=>i.type_id)]))].filter(id=>id>0).sort((a,b)=>a-b);
  const result: Prices = { items: {}, observed_at: "" };
  // Sequential batches stay inside the server's shared two-request budget.
  for (let i=0;i<ids.length;i+=20) {
    const value = await getData(`/api/v1/lp/prices?region=${encodeURIComponent(region)}&types=${ids.slice(i,i+20).join(",")}`,validPrices,signal);
    Object.assign(result.items,value.items);
    result.source = !result.source || result.source === value.source ? value.source : "ESI / Fuzzwork";
    if (!result.observed_at || value.observed_at < result.observed_at) result.observed_at = value.observed_at;
  }
  return result;
}

export function estimate(offer: Offer, prices: Record<string,Price>, mode: "sell"|"buy", tax: number) {
  const b = offer.blueprint;
  const product = b ? { type_id:b.product.type_id, quantity:b.product.quantity*offer.quantity } : { type_id:offer.type_id,quantity:offer.quantity };
  const materials = [...offer.required_items,...(b?.materials.map(m=>({...m,quantity:m.quantity*offer.quantity}))||[])];
  const unit = prices[product.type_id]?.[mode];
  const costReady = materials.every(m=>(prices[m.type_id]?.sell ?? 0)>0);
  const materialCost = costReady ? materials.reduce((sum,m)=>sum+prices[m.type_id].sell!*m.quantity,0) : null;
  const revenue = unit && product.quantity > 0 ? unit*product.quantity : null;
  const cost = materialCost === null ? null : offer.isk_cost+materialCost;
  const profit = revenue !== null && cost !== null && Number.isFinite(tax) && tax>=0 && tax<=100 ? revenue*(1-tax/100)-cost : null;
  // Offers charging both currencies cannot be ranked as a single LP or AK rate.
  const point = offer.lp_cost>0 && offer.ak_cost===0 ? "LP" : offer.ak_cost>0 && offer.lp_cost===0 ? "AK" : null;
  const efficiency = profit !== null && point ? profit/(point==="LP" ? offer.lp_cost : offer.ak_cost) : null;
  return { product, materials, materialCost, revenue, cost, profit, point, efficiency };
}
