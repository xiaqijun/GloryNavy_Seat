import { describe, expect, it } from "vitest";
import { estimate, validOffers, validPrices, type Offer } from "./api";
const offer: Offer = {offer_id:1,type_id:10,quantity:2,lp_cost:100,ak_cost:0,isk_cost:50,required_items:[{type_id:20,quantity:3}]};
describe("LP reference estimates",()=>{
  it("deducts sale tax, store cash and required materials at sell prices",()=>{
    const value=estimate(offer,{10:{buy:100,sell:200},20:{buy:1,sell:10}},"buy",5);
    expect(value.revenue).toBe(200);expect(value.cost).toBe(80);expect(value.profit).toBe(110);expect(value.efficiency).toBe(1.1);
  });
  it("uses blueprint runs and ME 0 materials instead of blueprint market price",()=>{
    const value=estimate({...offer,blueprint:{product:{type_id:30,quantity:5},materials:[{type_id:40,quantity:4}]}},{10:{buy:999,sell:999},20:{buy:1,sell:10},30:{buy:90,sell:100},40:{buy:1,sell:5}},"sell",0);
    expect(value.product.quantity).toBe(10);expect(value.cost).toBe(120);expect(value.profit).toBe(880);expect(value.efficiency).toBe(8.8);
  });
  it("does not treat missing materials, prices or manufacturing recipes as zero cost",()=>{
    expect(estimate(offer,{10:{buy:100,sell:200}},"sell",0).profit).toBeNull();
    expect(estimate(offer,{20:{buy:1,sell:10}},"sell",0).profit).toBeNull();
    expect(estimate({...offer,blueprint:{product:{type_id:0,quantity:0},materials:[]}},{10:{buy:100,sell:200},20:{buy:1,sell:10}},"sell",0).profit).toBeNull();
  });
  it("retains losses, validates tax and separates AK from LP",()=>{
    const prices={10:{buy:1,sell:2},20:{buy:1,sell:10}};
    expect(estimate(offer,prices,"sell",0).profit).toBe(-76);
    expect(estimate(offer,prices,"sell",NaN).profit).toBeNull();
    expect(estimate(offer,prices,"sell",101).profit).toBeNull();
    expect(estimate({...offer,lp_cost:0,ak_cost:2},prices,"sell",0).efficiency).toBe(-38);
    expect(estimate({...offer,ak_cost:2},prices,"sell",0).efficiency).toBeNull();
    expect(estimate({...offer,lp_cost:0},prices,"sell",0).efficiency).toBeNull();
  });
  it("rejects malformed response shapes",()=>{
    expect(validOffers({offers:[offer],names:{10:{zh:"测试",en:"Test"}},sde_build:1,corporation_id:1000130,observed_at:"2026-10-21T00:00:00Z"})).toBe(true);
    expect(validOffers({offers:[{...offer,lp_cost:-1}],names:{},sde_build:1,corporation_id:1000130,observed_at:""})).toBe(false);
    expect(validPrices({items:{10:{buy:null,sell:123}},observed_at:"2026-10-21T00:00:00Z"})).toBe(true);
    expect(validPrices({items:{10:{buy:NaN,sell:123}},observed_at:"2026-10-21T00:00:00Z"})).toBe(false);
  });
});
