import type { PhysicalContent } from "./catalog-api";
import type { Reward, Shop } from "./rewards-api";

export function maxRewardUnits(reward: Reward, shop: Shop): number {
  if (!reward.enabled || reward.coins_minor <= 0 || shop.isk_per_coin <= 0)
    return 0;
  const limits = [
    1000000,
    reward.stock,
    Math.floor(shop.available_minor / reward.coins_minor),
    Math.floor(Number.MAX_SAFE_INTEGER / Math.max(1, reward.isk_value)),
    Math.floor(1e9 / Math.max(1, reward.quantity)),
  ];
  if (reward.content) {
    limits.push(
      Math.floor(
        Number.MAX_SAFE_INTEGER / Math.max(1, reward.content.isk_minor || 0),
      ),
    );
    for (const entry of [...reward.content.items, ...reward.content.fittings])
      limits.push(Math.floor(1e9 / entry.quantity));
  }
  return Math.max(0, Math.min(...limits));
}

export function scaledReward(
  content: PhysicalContent | undefined,
  units: number,
): PhysicalContent | undefined {
  if (!content || !Number.isSafeInteger(units) || units < 1) return undefined;
  return {
    ...content,
    isk_minor: (content.isk_minor || 0) * units,
    fittings: content.fittings.map((f) => ({
      ...f,
      quantity: f.quantity * units,
    })),
    items: content.items.map((i) => ({ ...i, quantity: i.quantity * units })),
  };
}
