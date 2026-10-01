import { useQuery } from "@tanstack/react-query";
import { Plus, Settings2 } from "lucide-react";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { msg } from "@/lib/i18n";
import { RewardSummary } from "./growth-editor";
import * as api from "./api";

export function GrowthProject({
  corp,
  policy,
  characters,
  admin,
  apply,
  edit,
}: {
  corp: string;
  policy: api.Policy;
  characters: api.Character[];
  admin: boolean;
  apply: (character: string) => void;
  edit: () => void;
}) {
  const check = useQuery({
    queryKey: [
      "welfare",
      "growth-project",
      corp,
      policy.kind,
      policy.version,
      characters.map((c) => c.id),
    ],
    queryFn: async ({ signal }) => {
      let failure: unknown;
      // Try linked characters in order, stopping once a recipient qualifies.
      // Account-wide terminal states apply equally to the remaining characters.
      for (const character of characters) {
        try {
          const status = await api.growthCheck(
            corp,
            character.id,
            policy.kind,
            signal,
          );
          if (status.state === "met") return character.id;
          if (["claimed", "pending", "closed"].includes(status.state))
            return "";
        } catch (error) {
          if (signal.aborted) throw error;
          failure = error;
        }
      }
      if (failure) throw failure;
      return "";
    },
    refetchInterval: 30_000,
  });
  const rewards = policy.config.rewards;
  const title = api.projectLabel(policy.kind, policy.config);
  const singleNamedReward =
    rewards?.fittings.length === 1 &&
    rewards.items.length === 0 &&
    !rewards.coins_minor &&
    !rewards.isk_minor &&
    rewards.fittings[0].name === title;
  const configuration = admin && (
    <IconAction label={msg("项目配置")} onClick={edit}>
      <Settings2 size={18} />
    </IconAction>
  );
  return (
    <Card className="welfare-growth-project">
      {!singleNamedReward && (
        <header>
          <h3>{title}</h3>
          {configuration}
        </header>
      )}
      <div className="welfare-growth-project-content">
        <RewardSummary rewards={policy.config.rewards} compact />
        {!check.isError && check.data && (
          <Button onClick={() => apply(check.data)}>
            <Plus size={16} />
            {msg("申请")}
          </Button>
        )}
        {singleNamedReward && configuration}
      </div>
      {check.isError && (
        <div className="welfare-growth-project-error" role="alert">
          <span>{msg("资格检查失败")}</span>
          <Button variant="outline" onClick={() => void check.refetch()}>
            {msg("重试")}
          </Button>
        </div>
      )}
    </Card>
  );
}
