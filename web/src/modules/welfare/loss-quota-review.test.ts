import { describe, expect, it } from "vitest";
import { isQuotaReview, quotaLevel } from "./loss-quota-review";
const p = (cap: number, remaining = cap) => ({used_minor:cap-remaining,reserved_minor:0,cap_minor:cap,remaining_minor:remaining});
const q = {kind:"srp",daily:p(100),weekly:p(700),monthly:p(3000)};
describe("quota review",()=>{
 it("requires the correct tier and never passes the monthly ceiling",()=>{
  for(const [n,level] of [[100,""],[101,"weekly"],[700,"weekly"],[701,"monthly"],[3000,"monthly"],[3001,"blocked"]] as const) expect(quotaLevel(q,n)).toBe(level);
  expect(quotaLevel({...q,weekly:p(700,0)},1)).toBe("monthly");
  expect(quotaLevel({...q,monthly:{used_minor:0,reserved_minor:0}},10000)).toBe("monthly");
 });
 it("rejects malformed or incomplete quota responses",()=>{
  const v={policy_version:"1",case_version:"2",checked_at:"2026-10-19T00:00:00Z",quota:q};
  expect(isQuotaReview(v)).toBe(true);
  expect(isQuotaReview({...v,quota:{...q,daily:{used_minor:0,reserved_minor:0,cap_minor:100}}})).toBe(false);
  expect(isQuotaReview({...v,quota:{...q,weekly:p(100,101)}})).toBe(false);
  expect(isQuotaReview({...v,policy_version:1})).toBe(false);
 });
});
