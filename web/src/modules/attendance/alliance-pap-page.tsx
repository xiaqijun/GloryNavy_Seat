import { PAPRequirement } from "./pap-requirement";

/** Dedicated alliance PAP page; corporation PAP remains on the PAP tab. */
export function AlliancePAPPage({ user }: { user: string }) {
  return (
    <div className="attendance-section pap-page">
      <PAPRequirement user={user} settings />
    </div>
  );
}
