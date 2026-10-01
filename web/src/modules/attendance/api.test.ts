import { describe, it, expect } from "vitest";
import { isOnline, isDetail, isEvent, isChanged, duration, captureSummary } from "./api";
describe("attendance wire contract", () => {
  const event = {
    id: "1",
    corporation_id: "10",
    title: "Fleet",
    starts_at: "2026-09-15T00:00:00Z",
    state: "open",
    version: "1",
    participants: 1,
    can_manage: false,
  };
  const report = {
    since: "2026-09-09T00:00:00+08:00",
    until: "2026-09-15T00:00:00+08:00",
    estimated: true,
    seconds: 0,
    observed_characters: 0,
    days: [{ date: "2026-09-09", seconds: 0, samples: 0 }],
    members: [
      {
        user_id: "account",
        name: "Pilot",
        seconds: 0,
        samples: 0,
        characters: [
          { id: "123", name: "Pilot", state: "unknown", observed_at: null },
        ],
      },
    ],
  };
  it("accepts unknown states and no samples without inventing observed time", () => {
    expect(isOnline(report)).toBe(true);
    expect(isOnline({ ...report, estimated: false })).toBe(false);
  });
  it("accepts absent end times and validates recorded end times", () => {
    expect(isEvent(event)).toBe(true);
    expect(isEvent({ ...event, ends_at: null })).toBe(true);
    expect(isEvent({ ...event, state: "closed", ends_at: "2026-09-15T02:00:00Z" })).toBe(true);
    expect(isEvent({ ...event, ends_at: "invalid" })).toBe(false);
  });
  it("rejects corrupt durations and unknown upstream status strings", () => {
    expect(isOnline({ ...report, seconds: -1 })).toBe(false);
    expect(
      isOnline({
        ...report,
        days: [{ date: "2026-09-09", seconds: Infinity, samples: 2 }],
      }),
    ).toBe(false);
    expect(
      isOnline({
        ...report,
        members: [
          {
            ...report.members[0],
            characters: [
              { ...report.members[0].characters[0], state: "available" },
            ],
          },
        ],
      }),
    ).toBe(false);
  });
  it("keeps unbound attribution nullable and IDs as strings", () => {
    const detail = {
      event,
      entries: [
        {
          character_id: "123",
          name: "Pilot",
          account_id: null,
          source: "manual",
          present: false,
          recorded_at: "2026-09-15T00:00:00Z",
        },
      ],
    };
    expect(isDetail(detail)).toBe(true);
    expect(isDetail({ ...detail, event: { ...event, id: 1 } })).toBe(false);
  });
  it("validates writes and formats hours without asserting precision", () => {
    expect(isChanged({ event, recorded: 2, excluded: 1 })).toBe(true);
    expect(isChanged({ event, recorded: 2, excluded: -1 })).toBe(false);
    expect(duration(27000)).toBe("7.5 h");
  });
});

it("preserves legacy unknown exclusion reasons and displays new separate counts", () => {
  expect(captureSummary({ recorded: 2, excluded: 1 })).toBe(
    "已读取 2 名军团角色，跳过1 名（原因未记录）",
  );
  expect(
    captureSummary({
      recorded: 2,
      excluded: 1,
      excluded_external: 0,
      excluded_unbound: 1,
    }),
  ).toBe("已读取 2 名军团角色，跳过未绑定 1 名");
  expect(
    captureSummary({
      recorded: 2,
      excluded: 0,
      excluded_external: 0,
      excluded_unbound: 0,
    }),
  ).toBe("已读取 2 名军团角色");
});
