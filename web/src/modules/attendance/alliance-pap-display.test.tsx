import { describe, expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { PAPRequirement } from "./pap-requirement";
import { AlliancePAPMembers } from "./alliance-pap-members";
import type { AllianceReport } from "./alliance-pap-api";

const inventory = Array.from({ length: 5 }, (_, i) => ({
  character_id: i + 1,
  character_name: `Pilot ${i + 1}`,
}));
const retained: AllianceReport = {
  source: "alliance",
  month: "2026-10",
  available: true,
  complete: false,
  state: "error",
  points: 9.5,
  records_total: 1,
  last_synced_at: "2026-10-20T03:14:00Z",
  characters: [{ ...inventory[0], pap: 9.5 }],
  bound_characters: inventory,
  version: "1",
  can_manage: false,
};
const render = (report: AllianceReport) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["attendance", "pap-requirement", "member"], {
    source: "alliance",
    monthly_points: 3,
    version: "1",
    can_manage: false,
  });
  client.setQueryData(["attendance", "alliance-pap", "member"], report);
  const html = renderToStaticMarkup(
    <QueryClientProvider client={client}>
      <PAPRequirement user="member" />
    </QueryClientProvider>,
  );
  client.clear();
  return html;
};

describe("alliance PAP display", () => {
  it("shows all five bindings, the retained PAP and an explicit sync failure", () => {
    const html = render(retained);
    for (const binding of inventory)
      expect(html).toContain(binding.character_name);
    expect(html).toContain("9.5");
    expect(html.match(/暂无记录/g)).toHaveLength(4);
    expect(html).toContain("同步失败，显示上次成功数据");
    expect(html).not.toContain("暂无已绑定联盟 PAP");
  });
  it("does not equate a complete snapshot with meeting the PAP target", () => {
    const html = render({
      ...retained,
      state: "ready",
      complete: true,
      points: 1,
    });
    expect(html).toContain("未达标");
    expect(html).not.toContain("pap-status-pill is-complete");
  });
  it("counts all bound characters in the member list", () => {
    const html = renderToStaticMarkup(
      <AlliancePAPMembers
        loading={false}
        error={null}
        months={["2026-10"]}
        month="2026-10"
        onMonthChange={() => {}}
        retry={() => {}}
        report={{
          ...retained,
          target: 3,
          members: [
            {
              user_id: "member",
              name: "Nickname",
              points: 9.5,
              achieved: true,
              characters: retained.characters,
              bound_characters: inventory,
            },
          ],
        }}
      />,
    );
    expect(html).toContain("5 个角色");
    expect(html).toContain("Nickname");
  });
});
